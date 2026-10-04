// Text for the Help & Guide pages, one list per role. English source strings; the pages pass each
// through t(), and the Sinhala and Tamil text lives in helpGuideLabels.mjs.

/** The store manager's guide: what each part of the workspace does and when to use it. */
export const STORE_MANAGER_GUIDE = [
  {
    title: "Placing an order",
    items: [
      { title: "Fill in the order form", text: "Choose the brand, cooling need and the date you need the goods. Orders placed after 4 PM move to the next operating day; the countdown on the form shows how long is left." },
      { title: "Let the helper fill it in", text: "Open the order helper and paste or type the order the way you would tell a colleague, for example “rice 10 bags, oil 24 bottles”. It fills in the form for you to check. Nothing is sent until you press Submit Order." },
      { title: "Speak your order", text: "Press Speak your order and say the items and quantities. What you say appears in the box, after anything you already typed. Press Stop, check the text, then press Fill in for me. If the button is missing, your browser cannot listen; type the order instead." },
    ],
  },
  {
    title: "Following a delivery",
    items: [
      { title: "Arrival window", text: "Each order shows when it should arrive as a range, not one exact minute, with a delay-risk label beside it." },
      { title: "Arrival changes", text: "If the arrival moves by 30 minutes or more you get a notice with the old and new time. Smaller moves update quietly." },
      { title: "Moved to a later run", text: "If an order cannot go on today's trucks, the notice says why in plain words and when it is planned to go." },
    ],
  },
  {
    title: "Receiving the goods",
    items: [
      { title: "Confirm the receipt", text: "When the truck has delivered, open the receipt from Orders, enter what you received and confirm. If anything is short or damaged, choose the reason; a description is required for damage." },
      { title: "Report by the deadline", text: "Shortages must be reported within two working days of delivery so the dispatcher can still act. The deadline is shown on the receipt." },
      { title: "Temperature on arrival", text: "For chilled goods you can enter the temperature you measured as they came off the truck. It is optional, saved with the receipt and shown on the order's timeline next to the loading reading." },
    ],
  },
  {
    title: "Notifications",
    items: [
      { title: "The bell", text: "The bell at the top shows how many things need you: receipts to confirm, arrivals that moved by 30 minutes or more, orders moved to a later run, and messages sent to your store in the last two days. Open it to see the newest; See all opens the full list." },
      { title: "Text messages", text: "Urgent notices, such as a delay after a truck breakdown, are also sent to your store by text message if your store has agreed to texts. They appear in the bell as well." },
      { title: "Clearing items", text: "Messages can be marked as read. Receipts, arrival changes and deferrals clear by themselves once they are dealt with." },
    ],
  },
];

/** The dispatcher's guide to the newer screens and signals. */
export const DISPATCHER_GUIDE = [
  {
    title: "Planning ahead",
    items: [
      { title: "Ten-week forecast", text: "The forecast shows expected order volume for the next ten weeks by depot and brand. Each week has a likely range that gets wider the further ahead it is, because later weeks are less certain. Use it to plan vehicles, drivers and refrigerated capacity, not as confirmed orders." },
      { title: "Busy-day calendar", text: "The calendar covers the same ten weeks. Where the operating calendar has not been set yet, days follow the usual weekday pattern and the page says so." },
    ],
  },
  {
    title: "Live operations",
    items: [
      { title: "Road and weather risks", text: "Risks recorded on the Planning page (heavy rain, flooding, landslides, road closures) appear as a tag on every trip they may delay: by depot, by a district the trip still has to visit, or by trip, vehicle or plan reference. Dismissed risks are ignored and a severity you overrode is used instead. A high risk on an active trip is also added to Needs action." },
      { title: "Alerts grouped by trip", text: "When one trip has several problems at once, such as a closing window, no updates and chilled goods on board too long, they are shown together under the trip's most serious level. Acknowledge all clears the group; Open trip shows the trip." },
    ],
  },
  {
    title: "Notifications",
    items: [
      { title: "The bell", text: "The bell at the top counts the same open items as the Notifications page: breakdowns and temperature problems first, then loading shortfalls, wrong-vehicle reports, repeat deferrals, unsettled sync conflicts and store receipt issues. Items clear once they are resolved." },
      { title: "Breakdown texts to stores", text: "When you confirm a breakdown recovery, every store whose delivery now arrives 30 minutes or more later is sent a text message, if the store has agreed to texts." },
    ],
  },
  {
    title: "Loading and delivery records",
    items: [
      { title: "Typed order numbers", text: "Loaders scan labels to confirm a load. When a label cannot be scanned they may type the order number, but must give a reason (damaged label, will not scan, no scanner, other). Each trip shows how many lines were typed instead of scanned." },
      { title: "Temperature at receipt", text: "Stores can record the temperature of chilled goods on arrival. It appears on the order's timeline next to the loader's reading at departure." },
      { title: "Driver updates on weak signal", text: "The driver app sends stop updates before photos, so progress reaches you first and photos follow. A delivered stop is only recorded once its proof photo has arrived. Drivers can switch on low-data mode, which takes smaller photos and checks messages less often." },
    ],
  },
];
