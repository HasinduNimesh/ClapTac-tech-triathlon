#!/usr/bin/env python3
"""Copy an S3-compatible bucket to an empty restore bucket and verify every object."""

import hashlib
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET


def setting(name):
    value = os.environ.get(name, "").strip()
    if not value:
        raise RuntimeError(f"required setting {name} is missing")
    return value


def normalized_endpoint(value):
    parsed = urllib.parse.urlsplit(value)
    return urllib.parse.urlunsplit((parsed.scheme.lower(), parsed.netloc.lower(), parsed.path.rstrip("/"), "", ""))


def s3_request(endpoint, bucket, method, key="", query=None, body=b"", extra_headers=None, credentials=None):
    access_key, secret_key, region = credentials
    segments = [bucket, *key.split("/")] if key else [bucket]
    path = urllib.parse.urlsplit(endpoint).path.rstrip("/") + "/" + "/".join(
        urllib.parse.quote(segment, safe="-_.~") for segment in segments
    )
    pairs = sorted((query or {}).items())
    query_string = urllib.parse.urlencode(pairs, quote_via=urllib.parse.quote, safe="-_.~")
    url = endpoint.rstrip("/") + path + ("?" + query_string if query_string else "")
    parsed = urllib.parse.urlsplit(url)
    host = parsed.netloc
    now = time.strftime("%Y%m%dT%H%M%SZ", time.gmtime())
    date = now[:8]
    payload_hash = hashlib.sha256(body).hexdigest()
    headers = {name.lower(): " ".join(str(value).strip().split()) for name, value in (extra_headers or {}).items()}
    headers.update({"host": host, "x-amz-content-sha256": payload_hash, "x-amz-date": now})
    signed_names = sorted(headers)
    canonical_headers = "".join(f"{name}:{headers[name]}\n" for name in signed_names)
    canonical_query = "&".join(
        urllib.parse.quote(name, safe="-_.~") + "=" + urllib.parse.quote(value, safe="-_.~")
        for name, value in pairs
    )
    canonical_path = urllib.parse.quote(parsed.path, safe="/-_.~%")
    canonical_request = "\n".join((method, canonical_path, canonical_query, canonical_headers, ";".join(signed_names), payload_hash))
    scope = f"{date}/{region}/s3/aws4_request"
    string_to_sign = "\n".join(("AWS4-HMAC-SHA256", now, scope, hashlib.sha256(canonical_request.encode()).hexdigest()))
    signing_key = hmac_digest(hmac_digest(hmac_digest(hmac_digest(("AWS4" + secret_key).encode(), date), region), "s3"), "aws4_request")
    signature = hmac_digest(signing_key, string_to_sign).hex()
    authorization = (
        f"AWS4-HMAC-SHA256 Credential={access_key}/{scope}, "
        f"SignedHeaders={';'.join(signed_names)}, Signature={signature}"
    )
    request_headers = {name: value for name, value in (extra_headers or {}).items()}
    request_headers.update({
        "Host": host,
        "x-amz-content-sha256": payload_hash,
        "x-amz-date": now,
        "Authorization": authorization,
    })
    request = urllib.request.Request(url, data=body if method in ("PUT", "POST") else None, headers=request_headers, method=method)
    for attempt in range(30):
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                return response.status, response.headers, response.read()
        except urllib.error.HTTPError as error:
            return error.code, error.headers, error.read()
        except (urllib.error.URLError, OSError) as error:
            if attempt == 29:
                cause = error.reason if isinstance(error, urllib.error.URLError) else error
                raise RuntimeError(f"S3-compatible endpoint request failed ({type(cause).__name__})") from None
            time.sleep(1)


def hmac_digest(key, value):
    import hmac

    return hmac.new(key, value.encode() if isinstance(value, str) else value, hashlib.sha256).digest()


def list_page(endpoint, bucket, token, credentials):
    query = {"list-type": "2", "max-keys": "1000"}
    if token:
        query["continuation-token"] = token
    status, _, body = s3_request(endpoint, bucket, "GET", query=query, credentials=credentials)
    if status != 200:
        raise RuntimeError(f"could not list configured bucket (HTTP {status})")
    root = ET.fromstring(body)
    values = {child.tag.rsplit("}", 1)[-1]: child for child in root if child.tag.rsplit("}", 1)[-1] != "Contents"}
    objects = []
    for item in (child for child in root if child.tag.rsplit("}", 1)[-1] == "Contents"):
        fields = {child.tag.rsplit("}", 1)[-1]: child.text or "" for child in item}
        objects.append((fields["Key"], int(fields.get("Size", "0"))))
    truncated = values.get("IsTruncated") is not None and (values["IsTruncated"].text or "").lower() == "true"
    next_token = values.get("NextContinuationToken")
    if truncated and (next_token is None or not next_token.text):
        raise RuntimeError("source object listing is truncated without a continuation token")
    return objects, (next_token.text if truncated else "")


def ensure_bucket(endpoint, bucket, credentials):
    status, _, _ = s3_request(endpoint, bucket, "PUT", credentials=credentials)
    if status not in (200, 201, 204, 405, 409):
        raise RuntimeError(f"could not prepare restore bucket (HTTP {status})")


def object_metadata(headers):
    return {
        name.lower(): " ".join(value.strip().split())
        for name, value in headers.items()
        if name.lower() == "content-type" or name.lower().startswith("x-amz-meta-")
    }


def list_all(endpoint, bucket, credentials):
    objects = []
    token = ""
    while True:
        page, token = list_page(endpoint, bucket, token, credentials)
        objects.extend(page)
        if not token:
            return objects


def main():
    try:
        source_endpoint = setting("SOURCE_OBJECT_ENDPOINT")
        source_bucket = setting("SOURCE_OBJECT_BUCKET")
        restore_endpoint = setting("RESTORE_OBJECT_ENDPOINT")
        restore_bucket = setting("RESTORE_OBJECT_BUCKET")
        source_credentials = (
            setting("SOURCE_OBJECT_ACCESS_KEY"),
            setting("SOURCE_OBJECT_SECRET_KEY"),
            os.environ.get("SOURCE_OBJECT_REGION", "us-east-1"),
        )
        restore_credentials = (
            setting("RESTORE_OBJECT_ACCESS_KEY"),
            setting("RESTORE_OBJECT_SECRET_KEY"),
            os.environ.get("RESTORE_OBJECT_REGION", "us-east-1"),
        )
        if normalized_endpoint(source_endpoint) == normalized_endpoint(restore_endpoint) and source_bucket == restore_bucket:
            raise RuntimeError("source and restore locations must be different")

        ensure_bucket(restore_endpoint, restore_bucket, restore_credentials)
        target, token = list_page(restore_endpoint, restore_bucket, "", restore_credentials)
        if target or token:
            raise RuntimeError("restore bucket must be empty; refusing to overwrite existing objects")

        source_objects = list_all(source_endpoint, source_bucket, source_credentials)
        if not source_objects:
            raise RuntimeError("source bucket is empty; refusing a vacuous restore check")
        total_bytes = 0
        for key, _ in source_objects:
            status, headers, content = s3_request(source_endpoint, source_bucket, "GET", key, credentials=source_credentials)
            if status != 200:
                raise RuntimeError(f"source object read failed (HTTP {status})")
            content_headers = object_metadata(headers)
            status, _, _ = s3_request(restore_endpoint, restore_bucket, "PUT", key, body=content,
                                      extra_headers=content_headers, credentials=restore_credentials)
            if status not in (200, 201):
                raise RuntimeError(f"restore object write failed (HTTP {status})")
            status, restored_headers, restored = s3_request(restore_endpoint, restore_bucket, "GET", key, credentials=restore_credentials)
            if status != 200 or hashlib.sha256(content).digest() != hashlib.sha256(restored).digest():
                raise RuntimeError("restored object integrity check failed")
            if content_headers != object_metadata(restored_headers):
                raise RuntimeError("restored object metadata check failed")
            total_bytes += len(content)

        restored_objects = list_all(restore_endpoint, restore_bucket, restore_credentials)
        if sorted(source_objects) != sorted(restored_objects):
            raise RuntimeError("source and restored bucket inventories differ")
        if sorted(source_objects) != sorted(list_all(source_endpoint, source_bucket, source_credentials)):
            raise RuntimeError("source bucket changed during restore copy; retry against a quiescent snapshot")
        print(f"RESTORE=PASS objects={len(source_objects)} bytes={total_bytes} sha256=verified")
    except (RuntimeError, ET.ParseError, KeyError, ValueError) as error:
        print(f"RESTORE=FAIL reason={error}", file=sys.stderr)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
