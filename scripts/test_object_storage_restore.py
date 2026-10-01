import importlib.util
import unittest
from pathlib import Path
from unittest.mock import patch


spec = importlib.util.spec_from_file_location(
    "verify_object_storage_restore",
    Path(__file__).with_name("verify-object-storage-restore.py"),
)
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)


class ObjectStorageRestoreTests(unittest.TestCase):
    def test_list_page_parses_namespaced_objects_and_continuation_token(self):
        xml = b"""<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
          <IsTruncated>true</IsTruncated><NextContinuationToken>next-page-token</NextContinuationToken>
          <Contents><Key>proof/one</Key><Size>12</Size></Contents>
          <Contents><Key>proof/two</Key><Size>34</Size></Contents>
        </ListBucketResult>"""
        with patch.object(restore, "s3_request", return_value=(200, {}, xml)):
            objects, token = restore.list_page("https://source.test", "proof", "", ("reader", "secret", "us-east-1"))
        self.assertEqual(objects, [("proof/one", 12), ("proof/two", 34)])
        self.assertEqual(token, "next-page-token")

    def test_truncated_listing_without_token_fails_instead_of_skipping_objects(self):
        xml = b"<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>"
        with patch.object(restore, "s3_request", return_value=(200, {}, xml)):
            with self.assertRaisesRegex(RuntimeError, "without a continuation token"):
                restore.list_page("https://source.test", "proof", "", ("reader", "secret", "us-east-1"))

    def test_empty_nontruncated_listing_has_no_next_page(self):
        xml = b"<ListBucketResult><IsTruncated>false</IsTruncated></ListBucketResult>"
        with patch.object(restore, "s3_request", return_value=(200, {}, xml)):
            objects, token = restore.list_page("https://source.test", "proof", "", ("reader", "secret", "us-east-1"))
        self.assertEqual(objects, [])
        self.assertEqual(token, "")


if __name__ == "__main__":
    unittest.main()
