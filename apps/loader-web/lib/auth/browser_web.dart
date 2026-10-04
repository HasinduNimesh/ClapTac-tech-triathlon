import 'package:web/web.dart' as web;

import 'browser_base.dart';

/// The real browser (Flutter web).
class WebBrowser implements BrowserBridge {
  @override
  Uri get location => Uri.parse(web.window.location.href);

  @override
  String get origin => web.window.location.origin;

  @override
  void navigate(String url) => web.window.location.assign(url);

  @override
  void replaceUrl(String pathAndQuery) => web.window.history.replaceState(null, '', pathAndQuery);

  @override
  String? sessionGet(String key) {
    try {
      return web.window.sessionStorage.getItem(key);
    } catch (_) {
      return null; // storage blocked: the page still works, just without a remembered session
    }
  }

  @override
  void sessionSet(String key, String value) {
    try {
      web.window.sessionStorage.setItem(key, value);
    } catch (_) {}
  }

  @override
  void sessionRemove(String key) {
    try {
      web.window.sessionStorage.removeItem(key);
    } catch (_) {}
  }
}

BrowserBridge createBrowser() => WebBrowser();
