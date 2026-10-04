/// The few browser capabilities sign-in needs, behind an interface so the sign-in
/// logic can be tested without a browser.
abstract class BrowserBridge {
  /// The page's current URL.
  Uri get location;

  /// `scheme://host[:port]` of the page.
  String get origin;

  /// Leaves the app for [url] (a full page navigation, as a redirect to the identity server is).
  void navigate(String url);

  /// Replaces the address bar with [pathAndQuery] without reloading, so a callback's
  /// `?code=` does not stay in the history.
  void replaceUrl(String pathAndQuery);

  // Per-tab storage: it survives a reload but not closing the tab, which suits a
  // shared dock tablet.
  String? sessionGet(String key);
  void sessionSet(String key, String value);
  void sessionRemove(String key);
}
