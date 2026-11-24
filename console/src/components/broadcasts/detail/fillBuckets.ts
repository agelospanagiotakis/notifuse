import type { Dayjs } from 'dayjs'
import dayjs from '../../../lib/dayjs'
import { toNumber } from '../../web_analytics/lib/format'
import type { EngagementGranularity } from '../broadcastAnalytics'

const HOUR_MS = 3_600_000

export interface Bucket {
  /** The local hour or day of the workspace timezone, written as the server writes it: `…Z`. */
  key: string
  /** Null for a bucket still to come: the axis spans the whole period, and the future stays empty. */
  value: number | null
}

// `dayjs().tz()` misreads the hour around some daylight-saving changes when the machine's own
// zone differs from the target. `Intl` reads the same zone correctly, so the chart keys are built
// from it instead.
const zoneFormatters = new Map<string, Intl.DateTimeFormat>()
function zoneParts(instant: number, timezone: string) {
  let formatter = zoneFormatters.get(timezone)
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-GB', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23'
    })
    zoneFormatters.set(timezone, formatter)
  }
  const parts = formatter.formatToParts(new Date(instant))
  const value = (type: Intl.DateTimeFormatPartTypes) => parts.find((part) => part.type === type)?.value ?? ''
  return {
    year: value('year'),
    month: value('month'),
    day: value('day'),
    hour: value('hour') === '24' ? '00' : value('hour'),
    minute: value('minute'),
    second: value('second')
  }
}

/**
 * The key of the bucket an instant falls in. The server truncates `opened_at AT TIME ZONE tz`,
 * a timestamp without zone, and serialises it with a "Z": the local time, labelled UTC. Keys are
 * built the same way, and the server value is never converted, which would shift the chart by
 * the timezone's offset.
 */
export function bucketKey(instant: Dayjs | string, granularity: EngagementGranularity, timezone: string): string {
  const local = zoneParts(dayjs(instant).valueOf(), timezone)
  return granularity === 'hour'
    ? `${local.year}-${local.month}-${local.day}T${local.hour}:00:00Z`
    : `${local.year}-${local.month}-${local.day}T00:00:00Z`
}

/** The key of a bucket the server returned, read as text. */
export function serverBucketKey(value: unknown, granularity: EngagementGranularity): string | null {
  if (typeof value !== 'string') return null
  const match = /^(\d{4}-\d{2}-\d{2})(?:[T ](\d{2}))?/.exec(value)
  if (!match) return null
  return granularity === 'hour' ? `${match[1]}T${match[2] ?? '00'}:00:00Z` : `${match[1]}T00:00:00Z`
}

/**
 * Every bucket from the one of `start` to the one of `end`, with the server's value or 0, and null
 * for the buckets that start after now: « 30 d » of a broadcast sent 6 days ago shows its 30 days,
 * the last 24 empty rather than at 0. On a daylight-saving change an hour that does not exist locally has no bucket, and
 * an hour that occurs twice is one bucket, as the server groups it.
 */
export function fillBuckets({
  start,
  end,
  granularity,
  timezone,
  rows,
  column,
  measure,
  now = dayjs()
}: {
  start: Dayjs
  end: Dayjs
  granularity: EngagementGranularity
  timezone: string
  rows: Record<string, unknown>[]
  /** The bucket column (`opened_at_hour`). */
  column: string
  measure: string
  now?: Dayjs
}): Bucket[] {
  const values = new Map<string, number>()
  for (const row of rows) {
    const key = serverBucketKey(row[column], granularity)
    if (key) values.set(key, (values.get(key) ?? 0) + toNumber(row[measure]))
  }

  const last = end.valueOf()
  const nowMs = now.valueOf()
  const buckets: { key: string; future: boolean }[] = []
  const seen = new Set<string>()
  const push = (instant: Dayjs) => {
    const key = bucketKey(instant, granularity, timezone)
    if (!seen.has(key)) {
      seen.add(key)
      // The bucket under way (this hour, today) has its value so far; only later ones are empty.
      buckets.push({ key, future: instant.valueOf() > nowMs })
    }
  }
  if (granularity === 'hour') {
    // Absolute steps of one hour: dayjs steps in a timezone skip or repeat around a change.
    const startParts = zoneParts(start.valueOf(), timezone)
    const first = start.valueOf() - Number(startParts.minute) * 60_000 - Number(startParts.second) * 1000 - (start.valueOf() % 1000)
    for (let t = first; t <= last; t += HOUR_MS) push(dayjs.utc(t))
  } else {
    let day = dayjs(start).tz(timezone).format('YYYY-MM-DD')
    for (let t = dayjs.tz(day, timezone).valueOf(); t <= last; t = dayjs.tz(day, timezone).valueOf()) {
      push(dayjs.utc(t))
      day = dayjs.utc(day).add(1, 'day').format('YYYY-MM-DD')
    }
  }
  return buckets.map(({ key, future }) => ({ key, value: future ? null : (values.get(key) ?? 0) }))
}
