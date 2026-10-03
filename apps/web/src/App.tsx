import { AutomationsPage } from "./automations/AutomationsPage";
import { HabitHelper } from "./automations/HabitHelper";
import { lazy, Suspense } from "react";
import { Route, Routes } from "react-router-dom";
import { AuthProvider } from "./auth/AuthContext";
import { CallbackPage } from "./auth/CallbackPage";
import { RoleGate } from "./auth/RoleGate";
import { Layout } from "./components/Layout";
import { HomePage } from "./routes/HomePage";
import { LoginPage } from "./routes/LoginPage";
import { NotFoundPage } from "./routes/NotFoundPage";
import { LocaleProvider, useLocale } from "./i18n";

const DispatcherPage = lazy(() => import("./dispatcher/DispatcherPage").then((m) => ({ default: m.DispatcherPage })));
const AuditPage = lazy(() => import("./dispatcher/AuditPage").then((m) => ({ default: m.AuditPage })));
const MasterDataPage = lazy(() => import("./dispatcher/MasterDataPage").then((m) => ({ default: m.MasterDataPage })));
const OrderQueuePage = lazy(() => import("./dispatcher/OrderQueuePage").then((m) => ({ default: m.OrderQueuePage })));
const ForecastPage = lazy(() => import("./dispatcher/ForecastPage").then((m) => ({ default: m.ForecastPage })));
const PlanningPage = lazy(() => import("./dispatcher/PlanningPage").then((m) => ({ default: m.PlanningPage })));
const DriverPage = lazy(() => import("./driver/DriverPage").then((m) => ({ default: m.DriverPage })));
const DriverTripsPage = lazy(() => import("./driver/DriverTripsPage").then((m) => ({ default: m.DriverTripsPage })));
const LoadingPage = lazy(() => import("./loader/LoadingPage").then((m) => ({ default: m.LoadingPage })));
const LoaderPage = lazy(() => import("./loader/LoaderPage").then((m) => ({ default: m.LoaderPage })));
const NewOrderPage = lazy(() => import("./store-manager/NewOrderPage").then((m) => ({ default: m.NewOrderPage })));
const OrderListPage = lazy(() => import("./store-manager/OrderListPage").then((m) => ({ default: m.OrderListPage })));
const StoreManagerDashboardPage = lazy(() => import("./store-manager/StoreManagerDashboardPage").then((m) => ({ default: m.StoreManagerDashboardPage })));
const StoreManagerNotificationsPage = lazy(() => import("./store-manager/StoreManagerNotificationsPage").then((m) => ({ default: m.StoreManagerNotificationsPage })));
const TrackingPage = lazy(() => import("./store-manager/TrackingPage").then((m) => ({ default: m.TrackingPage })));
const StoreManagerLayout = lazy(() => import("./store-manager/StoreManagerLayout").then((m) => ({ default: m.StoreManagerLayout })));

function AppRoutes() {
  const { t } = useLocale();
  return (
    <Suspense fallback={<p role="status">{t("Loading screen…")}</p>}>
      <HabitHelper />
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<RoleGate role="STORE_MANAGER"><StoreManagerLayout /></RoleGate>}>
          <Route path="/store-manager" element={<StoreManagerDashboardPage />} />
          <Route path="/store-manager/automations" element={<AutomationsPage />} />
          <Route path="/store-manager/orders" element={<OrderListPage />} />
          <Route path="/store-manager/orders/new" element={<NewOrderPage />} />
          <Route path="/store-manager/tracking" element={<TrackingPage />} />
          <Route path="/store-manager/receipts" element={<TrackingPage receiptsOnly />} />
          <Route path="/store-manager/notifications" element={<StoreManagerNotificationsPage />} />
          <Route path="/store-manager/*" element={<NotFoundPage inWorkspace />} />
        </Route>
        <Route element={<Layout />}>
          <Route path="/" element={<HomePage />} />
          <Route path="/auth/callback" element={<CallbackPage />} />
          <Route path="/dispatcher" element={<RoleGate role="DISPATCHER"><DispatcherPage /></RoleGate>} />
          <Route path="/dispatcher/automations" element={<RoleGate role="DISPATCHER"><AutomationsPage /></RoleGate>} />
          <Route path="/dispatcher/orders" element={<RoleGate role="DISPATCHER"><OrderQueuePage /></RoleGate>} />
          <Route path="/dispatcher/planning" element={<RoleGate role="DISPATCHER"><PlanningPage /></RoleGate>} />
          <Route path="/dispatcher/audit" element={<RoleGate role="DISPATCHER"><AuditPage /></RoleGate>} />
          <Route path="/dispatcher/master-data" element={<RoleGate role="DISPATCHER"><MasterDataPage /></RoleGate>} />
          <Route path="/dispatcher/forecast" element={<RoleGate role="DISPATCHER"><ForecastPage /></RoleGate>} />
          <Route path="/loader" element={<RoleGate role="LOADER"><LoaderPage /></RoleGate>} />
          <Route path="/loader/loading" element={<RoleGate role="LOADER"><LoadingPage /></RoleGate>} />
          <Route path="/driver" element={<RoleGate role="DRIVER"><DriverPage /></RoleGate>} />
          <Route path="/driver/trips" element={<RoleGate role="DRIVER"><DriverTripsPage /></RoleGate>} />
          <Route path="*" element={<NotFoundPage />} />
        </Route>
      </Routes>
    </Suspense>
  );
}

export function App() {
  return <LocaleProvider><AuthProvider><AppRoutes /></AuthProvider></LocaleProvider>;
}
