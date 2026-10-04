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
const DispatcherLayout = lazy(() => import("./dispatcher/DispatcherLayout").then((m) => ({ default: m.DispatcherLayout })));
const LiveOperationsPage = lazy(() => import("./dispatcher/LiveOperationsPage").then((m) => ({ default: m.LiveOperationsPage })));
const FleetPage = lazy(() => import("./dispatcher/FleetPage").then((m) => ({ default: m.FleetPage })));
const DeferralHistoryPage = lazy(() => import("./dispatcher/DeferralHistoryPage").then((m) => ({ default: m.DeferralHistoryPage })));
const DispatcherNotificationsPage = lazy(() => import("./dispatcher/DispatcherNotificationsPage").then((m) => ({ default: m.DispatcherNotificationsPage })));
const DispatcherSettingsPage = lazy(() => import("./dispatcher/DispatcherSettingsPage").then((m) => ({ default: m.DispatcherSettingsPage })));
const MasterDataFrame = lazy(() => import("./dispatcher/DispatcherSettingsPage").then((m) => ({ default: m.MasterDataFrame })));
const AuditFrame = lazy(() => import("./dispatcher/DispatcherSettingsPage").then((m) => ({ default: m.AuditFrame })));
const DispatcherHelpPage = lazy(() => import("./dispatcher/DispatcherHelpPage").then((m) => ({ default: m.DispatcherHelpPage })));
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
const CreateDashboardPage = lazy(() => import("./store-manager/CreateDashboardPage").then((m) => ({ default: m.CreateDashboardPage })));
const ReceiptConfirmPage = lazy(() => import("./store-manager/ReceiptConfirmPage").then((m) => ({ default: m.ReceiptConfirmPage })));
const OrderTimelinePage = lazy(() => import("./store-manager/OrderEvidencePages").then((m) => ({ default: m.OrderTimelinePage })));
const TrackOrderPage = lazy(() => import("./store-manager/OrderEvidencePages").then((m) => ({ default: m.TrackOrderPage })));
const StoreSettingsPage = lazy(() => import("./store-manager/StoreSettingsPage").then((m) => ({ default: m.StoreSettingsPage })));
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
          <Route path="/store-manager/receipts" element={<ReceiptConfirmPage />} />
          <Route path="/store-manager/dashboards/new" element={<CreateDashboardPage />} />
          <Route path="/store-manager/orders/:orderId/timeline" element={<OrderTimelinePage />} />
          <Route path="/store-manager/orders/:orderId/track" element={<TrackOrderPage />} />
          <Route path="/store-manager/settings" element={<StoreSettingsPage />} />
          <Route path="/store-manager/notifications" element={<StoreManagerNotificationsPage />} />
          <Route path="/store-manager/*" element={<NotFoundPage inWorkspace />} />
        </Route>
        <Route element={<RoleGate role="DISPATCHER"><DispatcherLayout /></RoleGate>}>
          <Route path="/dispatcher" element={<DispatcherPage />} />
          <Route path="/dispatcher/orders" element={<OrderQueuePage />} />
          <Route path="/dispatcher/automations" element={<AutomationsPage />} />
          <Route path="/dispatcher/planning" element={<PlanningPage />} />
          <Route path="/dispatcher/live" element={<LiveOperationsPage />} />
          <Route path="/dispatcher/fleet" element={<FleetPage />} />
          <Route path="/dispatcher/deferrals" element={<DeferralHistoryPage />} />
          <Route path="/dispatcher/forecast" element={<ForecastPage />} />
          <Route path="/dispatcher/notifications" element={<DispatcherNotificationsPage />} />
          <Route path="/dispatcher/settings" element={<DispatcherSettingsPage />} />
          <Route path="/dispatcher/master-data" element={<MasterDataFrame><MasterDataPage /></MasterDataFrame>} />
          <Route path="/dispatcher/audit" element={<AuditFrame><AuditPage /></AuditFrame>} />
          <Route path="/dispatcher/help" element={<DispatcherHelpPage />} />
          <Route path="/dispatcher/*" element={<NotFoundPage inWorkspace workspace="dispatcher" />} />
        </Route>
        <Route element={<Layout />}>
          <Route path="/" element={<HomePage />} />
          <Route path="/auth/callback" element={<CallbackPage />} />
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
