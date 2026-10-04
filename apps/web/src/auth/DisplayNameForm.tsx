import { FormEvent, useEffect, useState } from "react";
import { useAuth } from "./AuthContext";
import { useLocale } from "../i18n";

export function DisplayNameForm() {
  const { profile, updateDisplayName } = useAuth();
  const { t } = useLocale();
  const [name, setName] = useState(profile?.displayName ?? "");
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  useEffect(() => { setName(profile?.displayName ?? ""); }, [profile?.displayName]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    const trimmed = name.trim();
    if (!trimmed || [...trimmed].length > 120) {
      setError(t("Enter a name of up to 120 characters."));
      return;
    }
    setSaving(true);
    try {
      await updateDisplayName(trimmed);
      setMessage(t("Name saved to your Waypoint account."));
    } catch {
      setError(t("Could not save your name. Try again."));
    } finally {
      setSaving(false);
    }
  }

  return <form className="dp-stack" onSubmit={save}>
    <label className="dp-field">{t("Your name")}
      <input value={name} onChange={(event) => setName(event.target.value)} maxLength={120} autoComplete="name" required />
    </label>
    <p className="muted">{t("Your name appears in your workspace and is saved across devices.")}</p>
    {error && <p className="status-bad" role="alert">{error}</p>}
    {message && <p className="status-ok" role="status">{message}</p>}
    <button type="submit" className="dp-btn" disabled={saving || !name.trim()}>{saving ? t("Saving…") : t("Save name")}</button>
  </form>;
}
