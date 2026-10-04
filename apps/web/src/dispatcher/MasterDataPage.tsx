import { FormEvent, useEffect, useState } from "react";
import { apiJSON, ApiError } from "../api/client";
import { useAuth } from "../auth/AuthContext";
import { todayLocal } from "../api/date";
import { useLocale } from "../i18n";
import { timeInputValue } from "./outletTime.mjs";
import { formatCoordinates, locationChange, openStreetMapLink, openStreetMapSearchLink } from "./outletLocation.mjs";

type Outlet = { id: string; brand: string; name: string; district: string; depot: string; dockType: string; parkingConstraint: string; mallWindow: boolean; windowOpenTime: string; windowCloseTime: string; accessInstructions:string; accessInstructionsUpdatedBy?:string; accessInstructionsUpdatedAt?:string; accessInstructionsConfirmedBy?:string; accessInstructionsConfirmedAt?:string; chilledTemperatureMinC?:number|null; chilledTemperatureMaxC?:number|null; latitude?:number; longitude?:number; locationApproximate?:boolean; version: number };
type CalendarDay = { date: string; isOperating: boolean; version: number };
type Policy = { version:number; cutoffLocalTime:string; deferralWeightPoints:number; maxDeferralCount:number; maxUnservedDays:number; maxTripsPerVehicle:number; createdBy?:string; createdAt?:string };
type Vehicle = {id:string;type:string;temp:string;weightCapacityKg:number;volumeCapacityM3:number;fuelType:string;kmPerL:number;weeklyFuelQuotaL:number;homeDepot:string;version:number};
type Incident = {id:string;vehicleId:string;date:string;tripId?:string;type:string;description:string;affectedStops:string[];status:string;reportedAt:string};
type NotificationPreferences = {outletId:string;phoneE164:string;consentEnabled:boolean;deferralsEnabled:boolean;majorDelaysEnabled:boolean;locale:string;version:number};

export function MasterDataPage() {
  const { user } = useAuth(); const { t } = useLocale(); const token = user?.access_token || "";
  const [outlets, setOutlets] = useState<Outlet[]>([]); const [selected, setSelected] = useState("");
  const [vehicles,setVehicles]=useState<Vehicle[]>([]);const [selectedVehicle,setSelectedVehicle]=useState("");
  const [day, setDay] = useState(""); const [isOperating, setIsOperating] = useState(true); const [dayVersion, setDayVersion] = useState(0);
  const [notice, setNotice] = useState(""); const [error, setError] = useState(""); const [saving, setSaving] = useState(false);
  const [policy,setPolicy]=useState<Policy|null>(null);const [policyDraft,setPolicyDraft]=useState<Policy|null>(null);const [policyPreview,setPolicyPreview]=useState<{examples:{priorDeferrals:number;daysSinceLastServed:number;priorityScore:number}[];hardConstraintsUnchanged:boolean}|null>(null);
  const [incidents,setIncidents]=useState<Incident[]>([]);const [incidentDate,setIncidentDate]=useState(todayLocal);const [incidentType,setIncidentType]=useState("breakdown");const [incidentTrip,setIncidentTrip]=useState("");const [incidentDescription,setIncidentDescription]=useState("");const [incidentStops,setIncidentStops]=useState("");
  const [notificationPrefs,setNotificationPrefs]=useState<NotificationPreferences|null>(null);
  const outlet = outlets.find((item) => item.id === selected);

  async function load() {
    if (!token) return;
    try { const [data,policyData,fleetData,incidentData] = await Promise.all([apiJSON<{ items: Outlet[] }>("/shared/outlets", token),apiJSON<{policy:Policy}>("/shared/policies/current",token),apiJSON<{items:Vehicle[]}>("/fleet/vehicles",token),apiJSON<{items:Incident[]}>("/fleet/incidents?openOnly=true",token)]); setOutlets(data.items || []); setSelected(current => current || data.items?.[0]?.id || "");setPolicy(policyData.policy);setPolicyDraft({...policyData.policy,cutoffLocalTime:policyData.policy.cutoffLocalTime.slice(0,5)});setVehicles(fleetData.items||[]);setSelectedVehicle(current=>current||fleetData.items?.[0]?.id||"");setIncidents(incidentData.items||[]); }
    catch (e) { setError(e instanceof Error ? e.message : t("Master data could not be loaded")); }
  }
  useEffect(() => { void load(); }, [token]);
  useEffect(()=>{if(!token||!selected)return;let active=true;setNotificationPrefs(null);void apiJSON<{preferences:NotificationPreferences}>(`/shared/outlets/${encodeURIComponent(selected)}/notification-preferences`,token).then(data=>{if(active)setNotificationPrefs(data.preferences);}).catch(e=>{if(!active)return;if(e instanceof ApiError&&e.status===404)setNotificationPrefs({outletId:selected,phoneE164:"",consentEnabled:false,deferralsEnabled:false,majorDelaysEnabled:false,locale:"en",version:0});else setError(e instanceof Error?e.message:t("Notification preferences could not be loaded"));});return()=>{active=false;};},[token,selected]);
  async function updateOutlet(e: FormEvent<HTMLFormElement>) {
    e.preventDefault(); if (!outlet) return; setSaving(true); setError(""); setNotice("");
    const form = new FormData(e.currentTarget);
    const place = locationChange(String(form.get("locationPair")||""), outlet);
    if (place.change === "invalid") { setError(t(place.message)); setSaving(false); return; }
    const minRaw=String(form.get("chilledTemperatureMinC")||"");const maxRaw=String(form.get("chilledTemperatureMaxC")||"");
    const body = { ...outlet, ...(place.change==='set'?{location:{latitude:place.latitude,longitude:place.longitude}}:place.change==='clear'?{location:null}:{}), name: String(form.get("name")||"").trim(), district: String(form.get("district")||"").trim(), depot: String(form.get("depot")||"").trim(), dockType: String(form.get("dockType")), parkingConstraint: String(form.get("parkingConstraint")), mallWindow: form.get("mallWindow")==="on", windowOpenTime: String(form.get("windowOpenTime")||""), windowCloseTime: String(form.get("windowCloseTime")||""), accessInstructions:String(form.get("accessInstructions")||"").trim(), chilledTemperatureMinC:minRaw===""?null:Number(minRaw),chilledTemperatureMaxC:maxRaw===""?null:Number(maxRaw) };
    try { const saved = await apiJSON<{outlet:Outlet}>(`/shared/outlets/${encodeURIComponent(outlet.id)}`,token,{method:"PUT",headers:{"If-Match":String(outlet.version)},body:JSON.stringify(body)}); setOutlets(items=>items.map(item=>item.id===saved.outlet.id?saved.outlet:item)); setNotice(`${t("Outlet")} ${saved.outlet.id} ${t("saved as version")} ${saved.outlet.version}; ${t("change recorded in audit history.")}`); }
    catch(e){setError(e instanceof ApiError&&e.status===409?t("This outlet changed in another session. Reload the latest version before editing."):e instanceof Error?e.message:t("Outlet update failed"));}
    finally{setSaving(false);}
  }
  async function saveNotificationPreferences(e:FormEvent<HTMLFormElement>){e.preventDefault();if(!notificationPrefs)return;setSaving(true);setError("");setNotice("");try{const result=await apiJSON<{preferences:NotificationPreferences}>(`/shared/outlets/${encodeURIComponent(selected)}/notification-preferences`,token,{method:"PUT",body:JSON.stringify(notificationPrefs)});setNotificationPrefs(result.preferences);setNotice(t("Outlet SMS preferences saved. Consent and setting changes are recorded in audit history."));}catch(e){setError(e instanceof ApiError&&e.status===409?t("These notification preferences changed in another session. Reload the latest version before editing."):e instanceof Error?e.message:t("Notification preferences could not be saved"));}finally{setSaving(false);}}
  async function loadCalendar() {
    if (!token || !day) return; setError(""); setNotice("");
    try { const data=await apiJSON<{items:CalendarDay[]}>(`/shared/calendar?from=${day}&to=${day}`,token); const found=data.items?.[0]; setDayVersion(found?.version||0); setIsOperating(found?.isOperating??true); }
    catch(e){setError(e instanceof Error?e.message:t("Calendar could not be loaded"));}
  }
  async function saveCalendar(e:FormEvent){e.preventDefault();if(!day)return;setSaving(true);setError("");setNotice("");
    try{const data=await apiJSON<{calendarDay:CalendarDay}>(`/shared/calendar/${day}`,token,{method:"PUT",body:JSON.stringify({isOperating,expectedVersion:dayVersion})});setDayVersion(data.calendarDay.version);setNotice(`${t("Operating calendar saved for")} ${day} ${t("as version")} ${data.calendarDay.version}; ${t("change recorded in audit history.")}`);}
    catch(e){setError(e instanceof ApiError&&e.status===409?t("This date changed in another session. Reload it before saving."):e instanceof Error?e.message:t("Calendar update failed"));}
    finally{setSaving(false);}
  }
  async function submitPolicy(previewOnly:boolean){if(!policyDraft)return;setSaving(true);setError("");setNotice("");
    const body={version:policyDraft.version,cutoffLocalTime:policyDraft.cutoffLocalTime,deferralWeightPoints:Number(policyDraft.deferralWeightPoints),maxDeferralCount:Number(policyDraft.maxDeferralCount),maxUnservedDays:Number(policyDraft.maxUnservedDays),maxTripsPerVehicle:Number(policyDraft.maxTripsPerVehicle)};
    try{if(previewOnly){const result=await apiJSON<{examples:{priorDeferrals:number;daysSinceLastServed:number;priorityScore:number}[];hardConstraintsUnchanged:boolean}>("/shared/policies/preview",token,{method:"POST",body:JSON.stringify(body)});setPolicyPreview(result);setNotice(t("Preview calculated. Hard vehicle, capacity, depot, cooling, delivery-window, and fuel constraints remain enforced."));}
      else{const result=await apiJSON<{policy:Policy}>("/shared/policies/current",token,{method:"PUT",body:JSON.stringify(body)});setPolicy(result.policy);setPolicyDraft({...result.policy,cutoffLocalTime:result.policy.cutoffLocalTime.slice(0,5)});setPolicyPreview(null);setNotice(`${t("Planning policy")} ${t("version")} ${result.policy.version} ${t("is active and recorded in audit history.")}`);}}
    catch(e){setError(e instanceof ApiError&&e.status===409?t("This planning policy changed in another session. Reload the latest version before saving."):e instanceof Error?e.message:t("Policy operation failed"));}finally{setSaving(false);}
  }
  async function updateVehicle(e:FormEvent<HTMLFormElement>){e.preventDefault();const current=vehicles.find(v=>v.id===selectedVehicle);if(!current)return;const form=new FormData(e.currentTarget);setSaving(true);setError("");setNotice("");
    const body={...current,type:String(form.get("type")),temp:String(form.get("temp")),weightCapacityKg:Number(form.get("weightCapacityKg")),volumeCapacityM3:Number(form.get("volumeCapacityM3")),fuelType:String(form.get("fuelType")).trim(),kmPerL:Number(form.get("kmPerL")),weeklyFuelQuotaL:Number(form.get("weeklyFuelQuotaL")),homeDepot:String(form.get("homeDepot")).trim()};
    try{const result=await apiJSON<{vehicle:Vehicle}>(`/fleet/vehicles/${encodeURIComponent(current.id)}/master-data`,token,{method:"PUT",body:JSON.stringify(body)});setVehicles(items=>items.map(v=>v.id===current.id?result.vehicle:v));setNotice(`${t("Vehicle")} ${current.id} ${t("saved as version")} ${result.vehicle.version}; ${t("audit delivery is queued and retries automatically if shared audit is offline.")}`);}
    catch(e){setError(e instanceof ApiError&&e.status===409?t("This vehicle changed in another session. Reload the current version before editing."):e instanceof Error?e.message:t("Vehicle update failed"));}finally{setSaving(false);}
  }
  async function reportIncident(e:FormEvent<HTMLFormElement>){e.preventDefault();const form=new FormData(e.currentTarget);setSaving(true);setError("");setNotice("");const body={vehicleId:String(form.get("vehicleId")),date:incidentDate,type:incidentType,tripId:incidentTrip.trim(),description:incidentDescription.trim(),affectedStops:incidentStops.split(/\n|,/).map(v=>v.trim()).filter(Boolean)};
    try{const result=await apiJSON<{incident:Incident}>("/fleet/incidents",token,{method:"POST",body:JSON.stringify(body)});setIncidents(items=>[result.incident,...items]);setIncidentTrip("");setIncidentDescription("");setIncidentStops("");setNotice(`${t("Incident saved.")} ${t("Vehicle")} ${result.incident.vehicleId} ${t("is unavailable for")} ${result.incident.date}; ${t("review replacement feasibility before confirming changes.")}`);}
    catch(e){setError(e instanceof Error?e.message:t("Incident could not be saved"));}finally{setSaving(false);}
  }

  return <section><h2>{t("Master data")}</h2><p>{t("Dispatcher changes are versioned, validated, and written to the audit history.")}</p>
    {error&&<p className="status-bad" role="alert">{error}</p>}{notice&&<p role="status">{notice}</p>}
    <h3>{t("Outlet access and delivery windows")}</h3>
    {outlets.length>0&&<p className={outlets.some(item=>item.locationApproximate!==false)?"status-warn":"status-ok"} role="status">{t("Outlets with an exact location")}: {outlets.filter(item=>item.locationApproximate===false).length} / {outlets.length}. {t("Drivers are navigated only to exact locations.")}</p>}
    <label>{t("Outlet")}<select value={selected} onChange={e=>setSelected(e.target.value)}>{outlets.map(item=><option key={item.id} value={item.id}>{item.id} · {item.brand} · {item.name}{item.locationApproximate===false?"":` · ${t("no exact location")}`}</option>)}</select></label>
    {outlet&&<form key={`${outlet.id}-${outlet.version}`} onSubmit={updateOutlet} className="grid">
      <p>{outlet.id} · {outlet.brand} · {t("version")} {outlet.version}</p>
      <label>{t("Outlet name")}<input name="name" defaultValue={outlet.name} maxLength={120} required/></label>
      <label>{t("District")}<input name="district" defaultValue={outlet.district} maxLength={80} required/></label>
      <label>{t("Depot")}<input name="depot" defaultValue={outlet.depot} maxLength={80} required/></label>
      <label>{t("Dock type")}<select name="dockType" defaultValue={outlet.dockType}><option value="normal">{t("Normal")}</option><option value="mall_dock">{t("Mall dock")}</option></select></label>
      <label>{t("Vehicle access")}<select name="parkingConstraint" defaultValue={outlet.parkingConstraint}><option value="normal">{t("Any eligible vehicle")}</option><option value="van_only">{t("Van only")}</option></select></label>
      <label>{t("Window opens")}<input name="windowOpenTime" type="time" defaultValue={timeInputValue(outlet.windowOpenTime)} /></label>
      <label>{t("Window closes")}<input name="windowCloseTime" type="time" defaultValue={timeInputValue(outlet.windowCloseTime)} /></label>
      <label><input name="mallWindow" type="checkbox" defaultChecked={outlet.mallWindow}/> {t("Mall delivery window")}</label>
      <fieldset><legend>{t("Delivery location")}</legend>
        <p className={outlet.locationApproximate===false?"status-ok":"status-warn"} role="status">{outlet.locationApproximate===false?t("Exact location recorded. Drivers are navigated to this point."):t("Only the approximate district position is known. Drivers are not navigated to it; they search by shop name until an exact location is recorded.")}</p>
        <label>{t("Exact location (latitude, longitude)")}<input name="locationPair" inputMode="decimal" autoComplete="off" placeholder="6.93441, 79.84281" defaultValue={outlet.locationApproximate===false&&outlet.latitude!=null&&outlet.longitude!=null?formatCoordinates(outlet.latitude,outlet.longitude):""}/></label>
        <p className="muted">{t("Open the shop in a map app, copy its latitude and longitude, and paste them here. Leave blank to keep the current position; clear a recorded position to go back to the approximate one.")}</p>
        <p>{outlet.locationApproximate===false&&outlet.latitude!=null&&outlet.longitude!=null?<a href={openStreetMapLink(outlet.latitude,outlet.longitude)} target="_blank" rel="noreferrer">{t("Check this point on OpenStreetMap")}</a>:<a href={openStreetMapSearchLink(outlet.name,outlet.district)} target="_blank" rel="noreferrer">{t("Find this shop on OpenStreetMap")}</a>}</p>
      </fieldset>
      <label>{t("Landmark, gate and last 200 metres instructions")}<textarea name="accessInstructions" defaultValue={outlet.accessInstructions} maxLength={1000} rows={4} placeholder={t("Describe the correct entrance, landmark and final approach")}/></label>
      <fieldset><legend>{t("Chilled delivery temperature limits (°C)")}</legend><p className="muted">{t("Optional outlet-specific review thresholds. Leave both blank until the outlet's accepted range is confirmed; this system does not set food-safety limits.")}</p><div className="row"><label>{t("Minimum °C")}<input name="chilledTemperatureMinC" type="number" min="-40" max="40" step="0.1" defaultValue={outlet.chilledTemperatureMinC??""}/></label><label>{t("Maximum °C")}<input name="chilledTemperatureMaxC" type="number" min="-40" max="40" step="0.1" defaultValue={outlet.chilledTemperatureMaxC??""}/></label></div></fieldset>
      {outlet.accessInstructions&&<p className={accessInstructionsNeedConfirmation(outlet)?"status-warn":"status-ok"} role="status">
        {accessInstructionsNeedConfirmation(outlet)?t("Store confirmation due — review the note with the outlet every 90 days."):t("Store confirmation current")}
        {outlet.accessInstructionsUpdatedBy&&outlet.accessInstructionsUpdatedAt&&<> · {t("Updated by")} {outlet.accessInstructionsUpdatedBy} · {formatSriLankaDate(outlet.accessInstructionsUpdatedAt)}</>}
        {outlet.accessInstructionsConfirmedBy&&outlet.accessInstructionsConfirmedAt&&<> · {t("Confirmed by")} {outlet.accessInstructionsConfirmedBy} · {formatSriLankaDate(outlet.accessInstructionsConfirmedAt)}</>}
      </p>}
      <button disabled={saving}>{t("Save outlet changes")}</button>
    </form>}
    {outlet&&notificationPrefs&&<><h4>{t("Low-bandwidth outlet alerts")}</h4><p>{t("SMS is sent only with recorded outlet consent. Major delay means an ETA at least 30 minutes later after a confirmed vehicle reassignment.")}</p><form key={`${notificationPrefs.outletId}-${notificationPrefs.version}`} onSubmit={saveNotificationPreferences} className="grid">
      <label>{t("Outlet SMS phone · E.164")}<input type="tel" inputMode="tel" autoComplete="tel" value={notificationPrefs.phoneE164} maxLength={16} placeholder="+94771234567" onChange={e=>setNotificationPrefs({...notificationPrefs,phoneE164:e.target.value})} required/></label>
      <label>{t("Message language")}<select value={notificationPrefs.locale} onChange={e=>setNotificationPrefs({...notificationPrefs,locale:e.target.value})}><option value="en">{t("English")}</option><option value="si">සිංහල</option><option value="ta">தமிழ்</option></select></label>
      <label><input type="checkbox" checked={notificationPrefs.consentEnabled} onChange={e=>setNotificationPrefs({...notificationPrefs,consentEnabled:e.target.checked})}/> {t("Outlet has consented to SMS alerts")}</label>
      <label><input type="checkbox" checked={notificationPrefs.deferralsEnabled} onChange={e=>setNotificationPrefs({...notificationPrefs,deferralsEnabled:e.target.checked})}/> {t("Send order deferral alerts")}</label>
      <label><input type="checkbox" checked={notificationPrefs.majorDelaysEnabled} onChange={e=>setNotificationPrefs({...notificationPrefs,majorDelaysEnabled:e.target.checked})}/> {t("Send major delay alerts")}</label>
      <p className="muted">{t("Consent can be withdrawn here. Saving creates an audited version; changing the number requires consent to be recorded again.")}</p>
      <button disabled={saving}>{t("Save SMS preferences")}</button>
    </form></>}
    <h3>{t("Fleet capabilities and weekly quotas")}</h3><label>{t("Vehicle")}<select value={selectedVehicle} onChange={e=>setSelectedVehicle(e.target.value)}>{vehicles.map(v=><option key={v.id} value={v.id}>{v.id} · {t(v.type)} · {v.homeDepot}</option>)}</select></label>
    {vehicles.find(v=>v.id===selectedVehicle)&&(()=>{const vehicle=vehicles.find(v=>v.id===selectedVehicle)!;return <form key={`${vehicle.id}-${vehicle.version}`} onSubmit={updateVehicle} className="grid"><p>{vehicle.id} · {t("version")} {vehicle.version} · {t("availability and workshop status are managed separately.")}</p>
      <label>{t("Vehicle type")}<select name="type" defaultValue={vehicle.type}><option value="truck">{t("Truck")}</option><option value="van">{t("Van")}</option></select></label>
      <label>{t("Temperature capability")}<select name="temp" defaultValue={vehicle.temp}><option value="ambient">{t("Ambient")}</option><option value="reefer">{t("Refrigerated")}</option></select></label>
      <label>{t("Weight capacity (kg)")}<input name="weightCapacityKg" type="number" min="1" step="0.1" defaultValue={vehicle.weightCapacityKg} required/></label>
      <label>{t("Volume capacity (m³)")}<input name="volumeCapacityM3" type="number" min="0.01" step="0.01" defaultValue={vehicle.volumeCapacityM3} required/></label>
      <label>{t("Fuel type")}<input name="fuelType" defaultValue={vehicle.fuelType} maxLength={40} required/></label>
      <label>{t("Efficiency (km/L)")}<input name="kmPerL" type="number" min="0.01" step="0.01" defaultValue={vehicle.kmPerL} required/></label>
      <label>{t("Weekly fuel quota (L)")}<input name="weeklyFuelQuotaL" type="number" min="0.01" step="0.01" defaultValue={vehicle.weeklyFuelQuotaL} required/></label>
      <label>{t("Home depot")}<input name="homeDepot" defaultValue={vehicle.homeDepot} maxLength={80} required/></label>
      <button disabled={saving}>{t("Save vehicle changes")}</button></form>;})()}
    <h3>{t("Vehicle incidents and breakdowns")}</h3>
    <form onSubmit={reportIncident} className="grid">
      <label>{t("Vehicle")}<select name="vehicleId" value={selectedVehicle} onChange={e=>setSelectedVehicle(e.target.value)} required>{vehicles.map(v=><option key={v.id} value={v.id}>{v.id} · {t(v.type)} · {v.homeDepot}</option>)}</select></label>
      <label>{t("Incident date")}<input type="date" value={incidentDate} onChange={e=>setIncidentDate(e.target.value)} required/></label>
      <label>{t("Incident type")}<select value={incidentType} onChange={e=>setIncidentType(e.target.value)}><option value="breakdown">{t("Vehicle breakdown")}</option><option value="accident">{t("Accident")}</option><option value="temperature_failure">{t("Cooling failure")}</option><option value="other">{t("Other")}</option></select></label>
      <label>{t("Trip reference (optional)")}<input value={incidentTrip} onChange={e=>setIncidentTrip(e.target.value)} maxLength={120}/></label>
      <label>{t("What happened")}<textarea value={incidentDescription} onChange={e=>setIncidentDescription(e.target.value)} maxLength={1000} required/></label>
      <label>{t("Affected outlet IDs (one per line)")}<textarea value={incidentStops} onChange={e=>setIncidentStops(e.target.value)} maxLength={2000}/></label>
      <button disabled={saving||vehicles.length===0}>{t("Record incident and block vehicle")}</button>
    </form>
    <ul>{incidents.map(item=><li key={item.id}><strong>{item.vehicleId} · {item.type}</strong> · {item.date}{item.tripId?` · ${item.tripId}`:""} · {item.description}{item.affectedStops.length?` · affected: ${item.affectedStops.join(", ")}`:""}</li>)}</ul>
    <h3>{t("Planning policy")} · {t("active version")} {policy?.version??t("loading")}</h3>
    {policyDraft&&<form className="grid" onSubmit={e=>{e.preventDefault();void submitPolicy(true);}}>
      <label>{t("Next-day cutoff · Sri Lanka time")}<input type="time" value={policyDraft.cutoffLocalTime} onChange={e=>setPolicyDraft({...policyDraft,cutoffLocalTime:e.target.value})} required/></label>
      <label>{t("Priority points per prior deferral")}<input type="number" min="1" max="60" value={policyDraft.deferralWeightPoints} onChange={e=>setPolicyDraft({...policyDraft,deferralWeightPoints:Number(e.target.value)})} required/></label>
      <label>{t("Deferral count cap")}<input type="number" min="1" max="30" value={policyDraft.maxDeferralCount} onChange={e=>setPolicyDraft({...policyDraft,maxDeferralCount:Number(e.target.value)})} required/></label>
      <label>{t("Unserved days cap")}<input type="number" min="30" max="730" value={policyDraft.maxUnservedDays} onChange={e=>setPolicyDraft({...policyDraft,maxUnservedDays:Number(e.target.value)})} required/></label>
      <label>{t("Maximum trips per vehicle")}<select value={policyDraft.maxTripsPerVehicle} onChange={e=>setPolicyDraft({...policyDraft,maxTripsPerVehicle:Number(e.target.value)})}><option value={1}>1 {t("trip")}</option><option value={2}>2 {t("trips")}</option></select></label>
      <div className="row"><button type="submit" disabled={saving}>{t("Preview policy")}</button><button type="button" disabled={saving} onClick={()=>void submitPolicy(false)}>{t("Save as new version")}</button></div>
      {policyPreview&&<div><p>{t("Scoring preview")} · {t("hard constraints unchanged")}: {policyPreview.hardConstraintsUnchanged?t("Yes"):t("No")}</p><table><thead><tr><th>{t("Prior deferrals")}</th><th>{t("Days since served")}</th><th>{t("Priority score")}</th></tr></thead><tbody>{policyPreview.examples.map((item,i)=><tr key={i}><td>{item.priorDeferrals}</td><td>{item.daysSinceLastServed}</td><td>{item.priorityScore}</td></tr>)}</tbody></table></div>}
      {policy&&<p className="muted">{t("Current")}: {t("cutoff")} {policy.cutoffLocalTime.slice(0,5)}, {t("version")} {policy.version}. {t("Saving creates a new immutable version.")}</p>}
    </form>}
    <h3>{t("Operating calendar")}</h3><form onSubmit={saveCalendar} className="row">
      <label>{t("Date")}<input type="date" value={day} onChange={e=>setDay(e.target.value)} required/></label>
      <button type="button" disabled={!day} onClick={()=>void loadCalendar()}>{t("Load date")}</button>
      <label><input type="checkbox" checked={isOperating} onChange={e=>setIsOperating(e.target.checked)}/> {t("Operating day")} · {t("version")} {dayVersion}</label>
      <button disabled={saving||!day}>{t("Save calendar")}</button>
    </form>
  </section>;
}

function accessInstructionsNeedConfirmation(outlet:Outlet):boolean {
  if(!outlet.accessInstructions.trim()) return false;
  const updated=outlet.accessInstructionsUpdatedAt?Date.parse(outlet.accessInstructionsUpdatedAt):NaN;
  const confirmed=outlet.accessInstructionsConfirmedAt?Date.parse(outlet.accessInstructionsConfirmedAt):NaN;
  return !Number.isFinite(confirmed)||!Number.isFinite(updated)||confirmed<updated||Date.now()-confirmed>90*24*60*60*1000;
}

function formatSriLankaDate(value:string):string {
  const date=new Date(value);
  return Number.isFinite(date.getTime())?new Intl.DateTimeFormat(undefined,{dateStyle:"medium",timeStyle:"short",timeZone:"Asia/Colombo"}).format(date):value;
}
