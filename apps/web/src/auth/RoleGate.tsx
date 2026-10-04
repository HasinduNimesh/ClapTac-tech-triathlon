import { Link } from "react-router-dom";
import { roleOf, useAuth } from "./AuthContext";
import { useLocale } from "../i18n";

export function RoleGate({ role, children }: { role: string; children: React.ReactNode }) {
  const { user, profile, profileLoading } = useAuth();
  const { t } = useLocale();

  if (profileLoading) return <p role="status">{t("Checking your access…")}</p>;
  if (import.meta.env.DEV && !user) return <>{children}</>;
  if (!user) return <section className="card"><h2>{t("Sign in required")}</h2><p><Link to="/login">{t("Sign in")}</Link> {t("to continue.")}</p></section>;
  if (!profile) return <p className="status-bad" role="alert">{t("Your access profile could not be loaded. Please try again.")}</p>;
  if (roleOf(profile)?.toUpperCase() !== role) {
    const roleName = role.split("_").map((part) => part[0] + part.slice(1).toLowerCase()).join(" ");
    return <section className="card"><h2>{t("Access denied")}</h2><p>{t("This page is for the")} {t(roleName)} {t("role.")}</p></section>;
  }

  return <>{children}</>;
}
