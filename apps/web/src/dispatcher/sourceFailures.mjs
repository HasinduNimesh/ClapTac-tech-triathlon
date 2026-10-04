// Which of a screen's data sources really failed. A source that answers 404 because the thing it
// looks up does not exist yet (the day's plan before one is built) has nothing to show but is not
// broken; only 5xx, network and other errors count as failures.
export function isSourceFailure(source) {
  if (!source || !source.error) return false;
  return !(source.missingIsEmpty && source.status === 404);
}

export function failedSourceCount(sources) {
  return sources.filter(isSourceFailure).length;
}
