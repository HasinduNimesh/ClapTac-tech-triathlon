const numbers = new Intl.Collator("en", { numeric: true, sensitivity: "base" });

/** Display only: requests, joins and React keys retain the original vehicle ID. */
export function vehicleNumber(id) {
  return String(id ?? "").replace(/_+/g, " ").trim();
}

function searchable(value) {
  return String(value ?? "").normalize("NFKC").toLowerCase().replace(/[\s_-]+/g, "");
}

export function browseVehicles(vehicles, query = "", sort = "number-asc") {
  const needle = searchable(query);
  const compareNumber = (a, b) => numbers.compare(vehicleNumber(a.id), vehicleNumber(b.id))
    || a.id.localeCompare(b.id, "en");
  return vehicles.filter((v) => searchable(v.id).includes(needle)).sort((a, b) => {
    if (sort === "number-desc") return -compareNumber(a, b);
    if (sort === "weight-desc" || sort === "volume-desc") {
      const key = sort === "weight-desc" ? "weightCapacityKg" : "volumeCapacityM3";
      const capacity = (v) => Number.isFinite(v[key]) ? v[key] : -Infinity;
      const difference = capacity(b) - capacity(a);
      if (difference) return difference;
    }
    return compareNumber(a, b);
  });
}
