// src/useSSE.js
// React hook that connects to the configui SSE endpoint (/api/v1/events)
// and maintains live IRC + rack-device status maps.
//
// Event batching: SSE events are buffered and flushed on the next animation
// frame (requestAnimationFrame).  This coalesces bulk events (e.g. 3600
// rack-device shutdowns during a leak) into one render pass with zero
// artificial delay — the flush happens at the earliest moment the browser
// can actually paint.

import { useEffect, useRef, useState, useCallback } from 'react';

const RECONNECT_BASE_MS = 1000; // initial reconnect delay
const RECONNECT_MAX_MS = 30000; // cap exponential backoff

/**
 * useSSE — provides real-time IRC status and rack-device status.
 *
 * Returns:
 *   {
 *     ircStatusMap:  { [ip]: { state, valve1, valve2 } },
 *     racdeviceBatch: Array<{ type, data }>  // batch of rack-device events (flushed each animation frame)
 *     connected: boolean,
 *   }
 */
export default function useSSE() {
  const [ircStatusMap, setIrcStatusMap] = useState({});
  const [racdeviceBatch, setRacdeviceBatch] = useState([]);
  const [connected, setConnected] = useState(false);

  const esRef = useRef(null);           // EventSource instance
  const reconnectTimer = useRef(null);
  const reconnectDelay = useRef(RECONNECT_BASE_MS);
  const mountedRef = useRef(true);

  // ── event batching refs ────────────────────────────────────────────
  // Events accumulate in refs (no re-render) and flush on requestAnimationFrame.
  const pendingIrcEvents = useRef([]);
  const ircFlushTimer = useRef(null);
  const pendingRacdeviceEvents = useRef([]);
  const racdeviceFlushTimer = useRef(null);

  // ── helpers ────────────────────────────────────────────────────────

  const applySnapshot = useCallback((data) => {
    const map = {};
    (data.ircs ?? []).forEach((irc) => {
      if (!irc.ip) return;
      map[irc.ip] = {
        state: irc.state ?? '',
        valve1: irc.valve1state ?? '',
        valve2: irc.valve2state ?? '',
      };
    });
    setIrcStatusMap(map);
  }, []);

  // Flush all queued IRC/valve events into a single setIrcStatusMap call
  const flushIrcEvents = useCallback(() => {
    const batch = pendingIrcEvents.current;
    pendingIrcEvents.current = [];
    ircFlushTimer.current = null;
    if (batch.length === 0) return;

    setIrcStatusMap((prev) => {
      let next = { ...prev };
      for (const { type, data } of batch) {
        if (!data.ip) continue;
        switch (type) {
          case 'irc_state_changed':
            next[data.ip] = {
              ...(next[data.ip] ?? {}),
              state: data.state ?? next[data.ip]?.state ?? '',
              valve1: next[data.ip]?.valve1 ?? '',
              valve2: next[data.ip]?.valve2 ?? '',
            };
            break;
          case 'valve_state_changed':
            // Use || (not ??) so that empty-string values preserve the
            // previous valve state.  The backend sends valve1state="" when
            // only valve2 changed, and vice-versa.
            next[data.ip] = {
              ...(next[data.ip] ?? {}),
              state: next[data.ip]?.state ?? '',
              valve1: data.valve1state || next[data.ip]?.valve1 || '',
              valve2: data.valve2state || next[data.ip]?.valve2 || '',
            };
            break;
          case 'irc_added':
            next[data.ip] = {
              state: data.state ?? '',
              valve1: '',
              valve2: '',
            };
            break;
          case 'irc_deleted':
            delete next[data.ip];
            break;
          default:
            break;
        }
      }
      return next;
    });
  }, []);

  // Queue an IRC/valve event and schedule a flush on the next animation frame
  const queueIrcEvent = useCallback((type, data) => {
    pendingIrcEvents.current.push({ type, data });
    if (!ircFlushTimer.current) {
      ircFlushTimer.current = requestAnimationFrame(flushIrcEvents);
    }
  }, [flushIrcEvents]);

  // Flush all queued rack-device events into a single setRacdeviceBatch call
  const flushRacdeviceEvents = useCallback(() => {
    const batch = pendingRacdeviceEvents.current;
    pendingRacdeviceEvents.current = [];
    racdeviceFlushTimer.current = null;
    if (batch.length === 0) return;
    setRacdeviceBatch(batch);
  }, []);

  // Queue a rack-device event and schedule a flush on the next animation frame
  const queueRacdeviceEvent = useCallback((evt) => {
    pendingRacdeviceEvents.current.push(evt);
    if (!racdeviceFlushTimer.current) {
      racdeviceFlushTimer.current = requestAnimationFrame(flushRacdeviceEvents);
    }
  }, [flushRacdeviceEvents]);

  // ── SSE connection management ──────────────────────────────────────

  const connect = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }

    const es = new EventSource('/api/v1/events', { withCredentials: true });
    esRef.current = es;

    es.addEventListener('snapshot', (e) => {
      try {
        const data = JSON.parse(e.data);
        applySnapshot(data);
        setConnected(true);
        reconnectDelay.current = RECONNECT_BASE_MS;
      } catch (err) {
        console.error('SSE snapshot parse error:', err);
      }
    });

    es.addEventListener('irc_state_changed', (e) => {
      try { queueIrcEvent('irc_state_changed', JSON.parse(e.data)); } catch {}
    });

    es.addEventListener('valve_state_changed', (e) => {
      try { queueIrcEvent('valve_state_changed', JSON.parse(e.data)); } catch {}
    });

    es.addEventListener('irc_added', (e) => {
      try { queueIrcEvent('irc_added', JSON.parse(e.data)); } catch {}
    });

    es.addEventListener('irc_deleted', (e) => {
      try { queueIrcEvent('irc_deleted', JSON.parse(e.data)); } catch {}
    });

    // Rack-device events — pass through to consumers for them to handle
    // (because they modify assocByIrc state which lives in IrcDashboard).
    // Events are buffered in a ref and flushed as a batch on the next
    // animation frame so that bulk updates (e.g. 3600 shutdowns) arrive
    // as one render pass.
    es.addEventListener('racdevice_added', (e) => {
      try { queueRacdeviceEvent({ type: 'racdevice_added', data: JSON.parse(e.data) }); } catch {}
    });

    es.addEventListener('racdevice_state_changed', (e) => {
      try { queueRacdeviceEvent({ type: 'racdevice_state_changed', data: JSON.parse(e.data) }); } catch {}
    });

    es.addEventListener('racdevice_deleted', (e) => {
      try { queueRacdeviceEvent({ type: 'racdevice_deleted', data: JSON.parse(e.data) }); } catch {}
    });

    es.addEventListener('heartbeat', () => {
      // Keep-alive — no action needed
    });

    es.onerror = () => {
      setConnected(false);
      es.close();
      esRef.current = null;

      // Schedule reconnect with exponential backoff
      const delay = reconnectDelay.current;
      reconnectDelay.current = Math.min(delay * 2, RECONNECT_MAX_MS);
      reconnectTimer.current = setTimeout(() => {
        if (mountedRef.current) connect();
      }, delay);
    };
  }, [applySnapshot, queueIrcEvent, queueRacdeviceEvent]);

  // ── lifecycle ──────────────────────────────────────────────────────

  useEffect(() => {
    mountedRef.current = true;
    connect();

    return () => {
      mountedRef.current = false;
      if (esRef.current) { esRef.current.close(); esRef.current = null; }
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      if (ircFlushTimer.current) cancelAnimationFrame(ircFlushTimer.current);
      if (racdeviceFlushTimer.current) cancelAnimationFrame(racdeviceFlushTimer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { ircStatusMap, racdeviceBatch, connected };
}
