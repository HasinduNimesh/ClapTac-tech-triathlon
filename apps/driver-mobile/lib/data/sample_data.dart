import 'driver_models.dart';

/// Placeholder content taken from the Figma frames. Real data comes from the
/// local database and sync queue once the screens are wired to them.
const sampleStops = <StopInfo>[
  StopInfo(
    sequence: 1,
    outletCode: 'OUT108',
    name: 'Dehiwala',
    windowStart: '08:00',
    windowEnd: '09:00',
    cartons: 12,
    accessNote: 'Rear entrance - Van-only access',
    contactNote: 'Call store on arrival',
    goods: 'Ambient cartons',
    orderRef: 'FR-4801',
  ),
  StopInfo(
    sequence: 2,
    outletCode: 'OUT061',
    name: 'Nugegoda',
    windowStart: '09:00',
    windowEnd: '10:00',
    cartons: 8,
    accessNote: 'Front loading bay',
    contactNote: 'Call store on arrival',
    goods: 'Ambient cartons',
    orderRef: 'FR-4811',
  ),
  StopInfo(
    sequence: 3,
    outletCode: 'OUT047',
    name: 'Kirulapone',
    windowStart: '10:00',
    windowEnd: '11:00',
    cartons: 120,
    accessNote: 'Rear entrance · van-only access',
    contactNote: 'Call the store on arrival · shared loading bay',
    goods: 'Chilled dairy',
    orderRef: 'FR-4821',
  ),
];

const sampleTrip = TripInfo(
  vehicleCode: 'VEH017',
  plate: 'WP LB-4521',
  tripRef: 'TRP02801',
  depot: 'Peliyagoda Depot',
  window: '08:00-11:00',
  stops: sampleStops,
);
