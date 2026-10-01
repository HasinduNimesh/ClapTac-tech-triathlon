import { Navigate } from "react-router-dom";

export function LoaderPage() {
  return <Navigate to="/loader/loading" replace />;
}
