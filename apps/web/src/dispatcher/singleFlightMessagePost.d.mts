type MessageRequest = { senderId: string; tripId: string; stopId?: string; body: string; token: string };
export function singleFlightMessagePost<Result>(
  post: (request: MessageRequest) => Promise<Result>,
): (request: MessageRequest) => Promise<Result>;
