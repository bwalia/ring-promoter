import { formatDistanceToNowStrict, format } from "date-fns";

// Go's zero time ("0001-01-01T00:00:00Z") marks "never"; treat anything before
// the epoch as missing.
export function isZeroTime(iso: string | undefined | null): boolean {
  if (!iso) return true;
  const d = new Date(iso);
  return isNaN(d.getTime()) || d.getFullYear() < 1971;
}

export function timeAgo(iso: string): string {
  if (isZeroTime(iso)) return "—";
  return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
}

export function absoluteTime(iso: string): string {
  if (isZeroTime(iso)) return "never";
  return format(new Date(iso), "PPpp");
}

export function duration(startIso: string, endIso?: string): string {
  const start = new Date(startIso).getTime();
  const end = endIso ? new Date(endIso).getTime() : Date.now();
  const secs = Math.max(0, Math.round((end - start) / 1000));
  if (secs < 60) return `${secs}s`;
  return `${Math.floor(secs / 60)}m ${secs % 60}s`;
}
