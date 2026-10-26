/** Browser uses same-origin paths; Docker and Vite proxy them to Domain API. */
export const API_URL = (import.meta.env.VITE_API_URL ?? '').replace(/\/$/, '')
