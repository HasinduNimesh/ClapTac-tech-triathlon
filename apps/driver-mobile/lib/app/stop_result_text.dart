/// What the end-of-day summary says under a stop that needs follow-up (a partial, failed or refused delivery):
/// how short it was, or the driver's note, and whether it is still only on this phone.
///
/// [stillOnPhone] is true while anything is waiting in the send queue. Once the queue is empty the server has
/// everything, so the text says so instead of claiming the update is unsent.
String stopFollowUpText({int? short, String note = '', required bool stillOnPhone}) {
  final where = stillOnPhone ? 'saved on this phone, not sent yet' : 'sent to Waypoint';
  final what = short != null && short > 0 ? '$short ${short == 1 ? 'unit' : 'units'} short' : note.trim();
  if (what.isEmpty) return stillOnPhone ? 'Saved on this phone, not sent yet' : 'Sent to Waypoint';
  return '$what - $where';
}
