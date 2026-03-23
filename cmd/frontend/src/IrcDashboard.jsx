
// src/IrcDashboard.jsx
import React, { useEffect, useMemo, useRef, useState, memo } from 'react';
import './App.css';

import {
  Button, Dialog, DialogTitle, Checkbox, DialogContent, TextField, DialogActions,
  Tooltip, CircularProgress, Typography, Collapse, IconButton,
  Table, TableHead, TableRow, TableCell, TableBody, Select, MenuItem
} from '@mui/material';

import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import AutorenewIcon from '@mui/icons-material/Autorenew';
import ErrorIcon from '@mui/icons-material/Error';
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty';
import WarningIcon from '@mui/icons-material/Warning';
import PowerSettingsNewIcon from '@mui/icons-material/PowerSettingsNew';
import DoneIcon from '@mui/icons-material/Done';
import Icon from '@mdi/react';
import { mdiValve, mdiValveOpen, mdiValveClosed, mdiHelpCircleOutline } from '@mdi/js';

import Avatar from '@mui/material/Avatar';
import Box from '@mui/material/Box';
import Divider from '@mui/material/Divider';

import AppBar from '@mui/material/AppBar';
import Toolbar from '@mui/material/Toolbar';
import AccountCircle from '@mui/icons-material/AccountCircle';
import LogoutIcon from '@mui/icons-material/Logout';
import Menu from '@mui/material/Menu';
// Optional: include change password in the dashboard menu
import ManageAccountsIcon from '@mui/icons-material/ManageAccounts';
import { changePassword, resetToDefault, authFetch } from './auth'; // only if you keep Change Password in the dashboard
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';
import FormControlLabel from '@mui/material/FormControlLabel';


const POLL_MS = 5000;

/** Shared poller for IRC live status - reduces duplicate API calls */
const useLiveStatusPoller = (ircList) => {
  const [statusMap, setStatusMap] = useState({});
  const abortRef = useRef(null);

  const fetchAndUpdate = async () => {
    if (abortRef.current) abortRef.current.abort();
    abortRef.current = new AbortController();

    try {
      const res = await authFetch('/api/v1/irc', { signal: abortRef.current.signal, cache: 'no-store' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();

      const newStatusMap = {};
      (data?.data ?? []).forEach(item => {
        const ip = item.ip ?? item.IP;
        if (!ip) return;
        newStatusMap[ip] = {
          state: item.state ?? item.State,
          valve1: item.valve1state ?? item.valve1Status ?? item.Valve1State ?? item.Valve1Status,
          valve2: item.valve2state ?? item.valve2Status ?? item.Valve2State ?? item.Valve2Status,
        };
      });
      setStatusMap(newStatusMap);
    } catch (err) {
      if (err.name !== 'AbortError') console.error('LiveStatus poller error:', err);
    }
  };

  useEffect(() => {
    // Only start polling when we have IRC systems to monitor
    if (ircList.length === 0) return;
    
    fetchAndUpdate();

    const intervalId = setInterval(() => {
      if (document.visibilityState === 'visible') fetchAndUpdate();
    }, POLL_MS);

    const onFocus = () => fetchAndUpdate();
    window.addEventListener('focus', onFocus);

    return () => {
      clearInterval(intervalId);
      window.removeEventListener('focus', onFocus);
      if (abortRef.current) abortRef.current.abort();
    };
  }, [ircList.length]);

  return statusMap;
};

/** Small “spinning then done” icon used in status */
const ShutdownIcon = () => {
  const [isSpinning, setIsSpinning] = useState(true);
  useEffect(() => {
    const timer = setTimeout(() => setIsSpinning(false), 3000);
    return () => clearTimeout(timer);
  }, []);
  return isSpinning ? (
    <AutorenewIcon
      style={{ color: '#42A5F5', animation: 'spin 1s linear infinite', transformOrigin: 'center' }}
    />
  ) : (
    <DoneIcon style={{ color: '#42A5F5' }} />
  );
};


/** Helpers for valve and system status visuals */
const VALVE_COLORS = { open: '#2e7d32', closed: '#e57373',opening: '#FFB300', closing: '#e57373', unknown: '#616161' };
const getValveStatusColor = (status) => {
  const s = (status || '').toLowerCase();
  if (s === 'open') return VALVE_COLORS.open;
  if (s === 'closed') return VALVE_COLORS.closed;
  if (s === 'opening') return VALVE_COLORS.opening;
  if (s === 'closing') return VALVE_COLORS.closing;
  return VALVE_COLORS.unknown;
};
const getValveIconPath = (status) => {
  const s = (status || '').toLowerCase();

  let path = mdiHelpCircleOutline;
  let className = '';

  if (s === 'open') {
    path = mdiValveOpen;
  } else if (s === 'closed') {
    path = mdiValveClosed;
  } else if (s === 'opening') {
    path = mdiValve;
    className = 'valve-spin-cw'; // animation for opening
  } else if (s === 'closing') {
    path = mdiValve;
    className = 'valve-spin-ccw'; // animation for closing
  }

  return { path, className };
};

const getSystemStatusTooltip = (status) => {
  switch (status) {
    case 'Power Status On':     return 'System is Power on';
    case 'Shutdown Received':   return 'System Received Shutdown';
    case 'Shutdown Failed':     return 'Shutdown command failed';
    case 'Power Status Off':    return 'System is Power off';
    case 'Running':             return 'System is connected';
    case 'Connection Failed':   return 'Unable to connect to system';
    default:                    return 'Status unknown';
  }
};
const getSystemStatusColor = (status) => {
  switch (status) {
    case 'Power Status On':     return '#66BB6A';
    case 'Power Status Off':    return '#42A5F5';
    case 'Shutdown Received':   return '#42A5F5';
    case 'Shutdown Failed':     return 'red';
    case 'Connection Failed':   return 'red';
    case 'Running':             return '#42A5F5';
    default:                    return 'gray';
  }
};
const getSystemStatusIcon = (status) => {
  switch (status) {
    case 'Power Status On':
      return <PowerSettingsNewIcon style={{ color: '#66BB6A' }} />;
    case 'Power Status Off':
        return <PowerSettingsNewIcon style={{ color: '#42A5F5' }} />;
    case 'Shutdown Received':
      return <ShutdownIcon />;
    case 'Shutdown Failed':
      return <ErrorIcon style={{ color: 'red' }} />;
    case 'Running':
      return <DoneIcon style={{ color: '#42A5F5' }} />;
    default:
      return <ErrorIcon style={{ color: 'gray' }} />;
  }
};

const getStatusColor = (status) => {
  switch (status) {
    case 'Monitoring':          return '#42A5F5';
    case 'Starting':            return '#ff9800';
    case 'Connection Failed':   return 'red';
    case 'Leak Detected':       return '#9c27b0';
    default:                    return 'gray';
  }
};
const getStatusIcon = (status) => {
  switch (status) {
    case 'Monitoring':
      return (
        <AutorenewIcon
          style={{ color: '#42A5F5', animation: 'spin 1s linear infinite', transformOrigin: 'center' }}
        />
      );
    case 'Starting':
      return <HourglassEmptyIcon style={{ color: '#ff9800' }} className="hourglass-rotate" />;
    case 'Connection Failed':
      return <ErrorIcon style={{ color: 'red' }} />;
    case 'Leak Detected':
      return <WarningIcon style={{ color: '#9c27b0' }} />;
    default:
      return <ErrorIcon style={{ color: 'gray' }} />;
  }
};

/** LiveStatus — displays IRC status using data from shared poller */
const LiveStatus = memo(function LiveStatus({
  ip,
  initialState,
  initialValve1,
  initialValve2,
  liveStatus,
}) {
  const [state, setState] = useState(initialState);
  const [valve1, setValve1] = useState(initialValve1);
  const [valve2, setValve2] = useState(initialValve2);
  const { path: valve1Path, className: valve1Class } = getValveIconPath(valve1);
  const { path: valve2Path, className: valve2Class } = getValveIconPath(valve2);

  // Update when shared poller provides new data
  useEffect(() => {
    if (!liveStatus) return;
    
    const isPresent = (v) => v !== undefined && v !== null && v !== '';
    
    if (isPresent(liveStatus.state)) setState((prev) => (liveStatus.state !== prev ? liveStatus.state : prev));
    if (isPresent(liveStatus.valve1)) setValve1((prev) => (liveStatus.valve1 !== prev ? liveStatus.valve1 : prev));
    if (isPresent(liveStatus.valve2)) setValve2((prev) => (liveStatus.valve2 !== prev ? liveStatus.valve2 : prev));
  }, [liveStatus]);

  return (
    <div style={{ flexGrow: 1, display: 'flex', alignItems: 'center', gap: '8px' }}>
      <span style={{ display: 'flex', alignItems: 'center', gap: '6px', color: 'black', fontWeight: 'normal' }}>| Status:</span>
      <span style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 'bold', color: getStatusColor(state) }}>
        {getStatusIcon(state)}
        {state ?? 'Unknown'}
      </span>
      <span>| Valve1 Status:</span>
      <span style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 'bold', color: getValveStatusColor(valve1) }}>
        <Icon path={valve1Path} size={0.9} className={valve1Class} />
        {valve1 ?? 'Unknown'}
      </span>
      <span>| Valve2 Status:</span>
      <span style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 'bold', color: getValveStatusColor(valve2) }}>
       <Icon path={valve2Path} size={0.9} className={valve2Class} />
        {valve2 ?? 'Unknown'}
      </span>
    </div>
  );
});

/** IrcRow — shows associated rack devices for an IRC and polls only their status */
function IrcRow({
  ircIp,
  expanded,
  assocByIrc,
  setAssocByIrc,
  selectedLinkedIPs,               // Set<string>
  onToggleLinkedIp,                // (ip: string) => void
  onToggleSelectAll,               // (checked: boolean) => void
}) {
  const controllerRef = useRef(null);
  const intervalRef   = useRef(null);

  useEffect(() => {
    if (!expanded) {
      if (intervalRef.current) { clearInterval(intervalRef.current); intervalRef.current = null; }
      if (controllerRef.current) { try { controllerRef.current.abort(); } catch {} controllerRef.current = null; }
      return;
    }

    const refreshStatusOnly = async () => {
      if (document.visibilityState === 'hidden') return;
      if (controllerRef.current) { try { controllerRef.current.abort(); } catch {} }
      const c = new AbortController();
      controllerRef.current = c;

      try {
        const res = await authFetch(`/api/v1/irc/${ircIp}/racdevice`, { signal: c.signal });
        if (!res.ok) return;
        const payload = await res.json();
        const incoming = Array.isArray(payload.data)
          ? payload.data.map(item => ({
              ip: item.ip ?? item.Ip ?? '',
              status: item.status || item.state || 'Unknown',
              type: (item.systemtypedesc || item.SystemTypeDesc || '').toUpperCase(),
              selectedType: item.type ?? null,
            }))
          : [];

        setAssocByIrc(prev => {
          const current = prev[ircIp] ?? [];
          if (current.length === 0) return { ...prev, [ircIp]: incoming };
          const byIp = new Map(incoming.map(it => [it.ip, it]));
          const merged = current.map(it => {
            const upd = byIp.get(it.ip);
            return upd ? { ...it, status: upd.status } : it;
          });
          return { ...prev, [ircIp]: merged };
        });
      } catch (e) {
        if (e.name !== 'AbortError') console.error('Status refresh failed for', ircIp, e);
      }
    };

    refreshStatusOnly();
    intervalRef.current = setInterval(refreshStatusOnly, POLL_MS);

    return () => {
      if (intervalRef.current) { clearInterval(intervalRef.current); intervalRef.current = null; }
      if (controllerRef.current) { try { controllerRef.current.abort(); } catch {} controllerRef.current = null; }
    };
  }, [expanded, ircIp, setAssocByIrc]);

  const rows = assocByIrc[ircIp] ?? [];
  
  // Derived select-all states for this IRC
  const totalLinked = Array.isArray(rows) ? rows.length : 0;
  const allLinkedSelected = totalLinked > 0 && selectedLinkedIPs.size === totalLinked;
  const someLinkedSelected = selectedLinkedIPs.size > 0 && selectedLinkedIPs.size < totalLinked;


  return (
    <Collapse in={expanded} timeout="auto" unmountOnExit>
      <div style={{ padding: '12px 16px', background: '#fafafa', borderTop: '1px solid #eee' }}>
        {rows.length > 0 ? (
          <Table size="small" style={{ borderCollapse: 'collapse', width: '100%' }}>
            <TableHead>
              <TableRow style={{ backgroundColor: '#f0f0f0' }}>
                <TableCell width={48}>
                  <Checkbox
                    size="small"
                    checked={allLinkedSelected}
                    indeterminate={someLinkedSelected}
                    onChange={(e) => onToggleSelectAll(e.target.checked)}
                    inputProps={{ 'aria-label': 'Select all linked systems' }}
                  />
                </TableCell>

                <TableCell><strong>Rack Device </strong></TableCell>
                <TableCell><strong>Type</strong></TableCell>
                <TableCell><strong>Status</strong></TableCell>
              </TableRow>
            </TableHead>

            <TableBody>
              {rows.map((sys, i) => (
                <TableRow
                  key={`${sys.ip}-${i}`}
                  style={{ cursor: 'pointer', transition: 'background 0.3s' }}
                  onMouseEnter={(e) => (e.currentTarget.style.background = '#e6f7ff')}
                  onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
                >
                  <TableCell width={48}>
                    <Checkbox
                      size="small"
                      checked={selectedLinkedIPs.has(sys.ip)}
                      onChange={() => onToggleLinkedIp(sys.ip)}
                      inputProps={{ 'aria-label': `Select ${sys.ip}` }}
                    />
                  </TableCell>

                  <TableCell>
                    <a
                      href={`http://${sys.ip}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ color: '#1976d2', textDecoration: 'none' }}
                    >
                      {sys.ip}
                    </a>
                  </TableCell>

                  <TableCell>{sys.type}</TableCell>
                  <TableCell>
                    <Tooltip title={getSystemStatusTooltip(sys.status)}>
                      <span style={{ display: 'flex', alignItems: 'center', gap: '6px', fontWeight: 'bold', color: getSystemStatusColor(sys.status) }}>
                        {getSystemStatusIcon(sys.status)}
                        {sys.status === 'Running' ? 'Connected' : sys.status}
                      </span>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <Typography variant="body2" color="textSecondary">
            No associated rack devices added yet.
          </Typography>
        )}
      </div>
    </Collapse>

  );
}

/** IrcDashboard — main dashboard component (all its hooks are at top-level) */
export default function IrcDashboard({ onLogout }) {
  const [ircList, setIrcList] = useState([]);
  const [systemTypes, setSystemTypes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  
  // Use shared poller for IRC live status - pass ircList to control when polling starts
  const liveStatusMap = useLiveStatusPoller(ircList);

  const [selectedIPs, setSelectedIPs] = useState(() => new Set());
  const [bulkConfirmOpen, setBulkConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [errorMsg, setErrorMsg] = useState('');

  const [openIRCDialog, setOpenIRCDialog] = useState(false);
  const [newIRC, setNewIRC] = useState({ ip: '', username: '', password: '' });

  const [openSystemDialog, setOpenSystemDialog] = useState(false);
  const [newSystem, setNewSystem] = useState({ ip: '', username: '', password: '', systemtypedesc: '' , isforcefulshutdown: '', timeout: ''});
  const [selectedIRC, setSelectedIRC] = useState(null);
  
  
// Remove the old single-set if you no longer use it anywhere:
// const [selectedLinkedIPs, setSelectedLinkedIPs] = React.useState(new Set());

// Dialog & progress for linked deletion (these are fine)
const [linkedConfirmOpen, setLinkedConfirmOpen] = React.useState(false);
const [deletingLinked, setDeletingLinked] = React.useState(false);
const [linkedErrorMsg, setLinkedErrorMsg] = React.useState('');

// Selected linked systems per IRC (JS object: { [ircIp]: Set<string> })
const [selectedLinkedByIrc, setSelectedLinkedByIrc] = React.useState({});

// Toggle a single linked IP for an IRC
const toggleLinkedIp = (ircIp, ip) => {
  setSelectedLinkedByIrc(prev => {
    const next = { ...prev };
    const set = new Set(next[ircIp] ?? []);
    if (set.has(ip)) set.delete(ip);
    else set.add(ip);
    next[ircIp] = set;
    return next;
  });
};

// Toggle Select-All for an IRC
const toggleSelectAllLinked = (ircIp, checked) => {
  setSelectedLinkedByIrc(prev => {
    const next = { ...prev };
    const rows = assocByIrc[ircIp] ?? [];
    next[ircIp] = checked ? new Set(rows.map(r => r.ip)) : new Set();
    return next;
  });
};

// Get all selected linked IPs across every IRC
const getAllSelectedLinkedIPs = () => {
  const all = [];
  for (const ircIp of Object.keys(selectedLinkedByIrc)) {
    const set = selectedLinkedByIrc[ircIp];
    if (set && set.size > 0) {
      set.forEach(ip => all.push(ip));
    }
  }
  return all;
};

// Helper: is any linked system selected globally?
const anyLinkedSelected = Object.values(selectedLinkedByIrc).some(set => (set?.size ?? 0) > 0);


// Add IRC dialog field errors
  const [ircErrors, setIrcErrors] = useState({ ip: '', username: '', password: '' });

  // Add Rack Devices dialog field errors
  const [sysErrors, setSysErrors] = useState({
    ip: '',
    username: '',
    password: '',
    systemtypedesc: '',
    timeout: '', // only used when graceful
  });

  const [expanded, setExpanded] = useState({});
  const [assocByIrc, setAssocByIrc] = useState({});
  const [openScriptDialog, setOpenScriptDialog] = useState(false);
  const [scriptFile, setScriptFile] = useState(null);


  
// Profile menu state
  const [profileAnchor, setProfileAnchor] = useState(null);
  const profileMenuOpen = Boolean(profileAnchor);
  const openProfileMenu = (e) => setProfileAnchor(e.currentTarget);
  const closeProfileMenu = () => setProfileAnchor(null);

  // (Optional) Change Password dialog managed here on the dashboard
  
// Confirm Reset dialog state (add these)
const [openResetConfirm, setOpenResetConfirm] = useState(false);
const [resetBusy, setResetBusy] = useState(false);
const [resetError, setResetError] = useState('');

  const [openChange, setOpenChange] = useState(false);
  const [oldPass, setOldPass] = useState('');
  const [newPass, setNewPass] = useState('');
  const [confirm, setConfirm] = useState('');
  const [cpError, setCpError] = useState('');
  const [cpBusy, setCpBusy] = useState(false);

  const [typeOpen, setTypeOpen] = useState(false);


  

  
// --- validators (put these near the top of the file) ---
  const IPV4_RE = /^[a-zA-Z0-9:.]+$/;

  const isValidIPv4 = (ip) => typeof ip === 'string' && IPV4_RE.test(ip.trim());
  const isNonEmpty = (s) => typeof s === 'string' && s.trim().length > 0;
  const hasMinLen = (s, n) => typeof s === 'string' && s.trim().length >= n;   // ← define this
  const isNonNegativeIntegerString = (s) => {
    if (!isNonEmpty(s)) return false;
    const n = Number(s);
    return Number.isInteger(n) && n >= 0;
  };

// --- Change Password dialog handlers ---


  const closeChange = () => {
    if (cpBusy) return;
    setOpenChange(false);
    setOldPass(''); setNewPass(''); setConfirm(''); setCpError('');
  };

  const saveChange = async () => {
    setCpError('');
    if (!hasMinLen(newPass, 4)) {
      setCpError('Password must be at least 4 characters.');
      return;
    }
    if (newPass !== confirm) {
      setCpError('New password and confirmation do not match.');
      return;
    }
    setCpBusy(true);
    const res = await changePassword(oldPass, newPass);
    setCpBusy(false);
    if (!res.ok) {
      setCpError(res.message || 'Unable to change password.');
      return;
    }
    closeChange();
  };

  // Fetch IRC list on mount
  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        const res = await authFetch('/api/v1/irc');
        if (!res.ok) throw new Error('Failed to fetch IRC list');
        const data = await res.json();
        setIrcList(data.data || []);
        setSystemTypes(data.systemTypes || []);
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  const refreshIRCList = async (expandIp) => {
    try {
      const res = await authFetch('/api/v1/irc');
      const data = await res.json();
      setIrcList(data.data || []);
      setSystemTypes(data.systemTypes || []);
      // if (expandIp) setExpanded(prev => ({ ...prev, [expandIp]: true }));
    } catch (err) {
      console.error('Error refreshing IRC list:', err);
    }
  };

  const handleAddIRC = async () => {
    try {
      await authFetch('/api/v1/irc', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(newIRC),
      });
      setOpenIRCDialog(false);
      setNewIRC({ ip: '', username: '', password: '' });
      refreshIRCList();
    } catch (err) {
      console.error('Error adding IRC:', err);
    }
  };

  const toggleSelect = (ip) => {
    setSelectedIPs(prev => {
      const next = new Set(prev);
      next.has(ip) ? next.delete(ip) : next.add(ip);
      return next;
    });
  };

  
const handleAddSystem = async () => {
  if (!selectedIRC) return;
  try {
    // Normalize values and keys to what your API expects
    const payload = {
      ip: String(newSystem.ip || '').trim(),
      username: String(newSystem.username || '').trim(),
      password: String(newSystem.password || '').trim(),
      systemtypedesc: newSystem.systemtypedesc ?? '',
      // Your API expects these names:
      isforcefulshutdown: Boolean(newSystem.isforcefulshutdown),
      timeout: newSystem.isforcefulshutdown === false
        ? Number.isFinite(Number(newSystem.timeout)) ? Number(newSystem.timeout) : 300
        : 0, // or omit if your API allows missing timeout when forceful
    };

    // Basic validation before sending
    if (!payload.ip || !payload.username || !payload.password || !payload.systemtypedesc) {
      console.error('Missing required fields (ip, username, password, systemtypedesc).');
      return;
    }
    if (typeof payload.isforcefulshutdown !== 'boolean') {
      console.error('isforcefulshutdown must be a boolean.');
      return;
    }
    if (payload.isforcefulshutdown === false && (!Number.isFinite(payload.timeout) || payload.timeout < 0)) {
      console.error('timeout must be a non-negative integer when graceful.');
      return;
    }

    console.log('Sending payload:', payload);

    const res = await authFetch(`/api/v1/irc/${selectedIRC}/racdevice`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    // Log server response for easier debugging
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      console.error('Server responded with', res.status, text);
      return;
    }

    // Close dialog and reset state to your shape
    setOpenSystemDialog(false);
    setNewSystem({
      ip: '',
      username: '',
      password: '',
      systemtypedesc: '',
      isforcefulshutdown: true,  // or undefined if you want a blank radio state
      timeout: 0,                 // or '' if you want the field empty
    });

    await refreshIRCList(selectedIRC);
    await onExpand(selectedIRC, true);
  } catch (err) {
    console.error('Error adding system:', err);
  }
};


 
async function bulkDeleteLinkedSystems(ipsByIrc) {
  const failed = {};

  for (const [ircIp, ips] of Object.entries(ipsByIrc || {})) {
    if (!Array.isArray(ips) || ips.length === 0) continue;

    const res = await authFetch(`/api/v1/irc/${encodeURIComponent(ircIp)}/racdevice`, {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ips }),
    });

    if (!res.ok) {
      const text = await res.text().catch(() => '');
      for (const ip of ips) {
        failed[ip] = `HTTP ${res.status} — ${text}`;
      }
      continue;
    }

    const payload = await res.json().catch(() => ({}));
    if (payload && payload.failed && typeof payload.failed === 'object') {
      Object.assign(failed, payload.failed);
    }
  }

  return Object.keys(failed).length > 0 ? { failed } : {};
}

const handleDeleteLinkedSystems = async () => {
  const ips = getAllSelectedLinkedIPs(); // <-- your helper that returns string[]
  if (ips.length === 0) return;

  setDeletingLinked(true);
  setLinkedErrorMsg('');

  // Build lookup: which IRC contains which linked IP (based on current assocByIrc)
  const linkedIpToIrc = new Map();
  for (const ircIp of Object.keys(assocByIrc)) {
    const rows = assocByIrc[ircIp] ?? [];
    for (const r of rows) {
      linkedIpToIrc.set(r.ip, ircIp);
    }
  }

  // Snapshot previous assoc to roll back (deep copy)
  // Use structuredClone if available (modern browsers) or JSON fallback:
  const prevAssoc =
    typeof structuredClone === 'function'
      ? structuredClone(assocByIrc)
      : JSON.parse(JSON.stringify(assocByIrc));

  // Optimistic: remove selected IPs from each IRC's list
  setAssocByIrc(prev => {
    const next = { ...prev };  // Copy existing state
    for (const ircIp of Object.keys(selectedLinkedByIrc)) {  // Only IRCs with deletions
      const selectedSet = selectedLinkedByIrc[ircIp] ?? new Set();
      const rows = prev[ircIp] ?? [];
      next[ircIp] = rows.filter(r => !selectedSet.has(r.ip));
    }
    return next;  // Return updated copy
  });

  try {
    const ipsByIrc = {};
    for (const [ircIp, selectedSet] of Object.entries(selectedLinkedByIrc || {})) {
      const list = Array.from(selectedSet ?? []);
      if (list.length > 0) ipsByIrc[ircIp] = list;
    }

    const result = await bulkDeleteLinkedSystems(ipsByIrc);

    // Roll back only failed IPs to correct IRC
    if (result && result.failed && Object.keys(result.failed).length > 0) {
      const failedIPs = Object.keys(result.failed);

      // Log error for each failed deletion
      failedIPs.forEach(ip => {
        console.error(`Delete failed for ${ip}: ${result.failed[ip]}`);
      });

      // Reinsert each failed IP back into its IRC list from prev snapshot
      setAssocByIrc(prev => {
        const next = { ...prev };
        for (const ip of failedIPs) {
          const ircIp = linkedIpToIrc.get(ip);
          if (!ircIp) continue;

          const originalRow = (prevAssoc[ircIp] ?? []).find(r => r.ip === ip);
          if (!originalRow) continue;

          // Put it back if missing
          const currentRows = next[ircIp] ?? [];
          const exists = currentRows.some(r => r.ip === ip);
          if (!exists) {
            next[ircIp] = [...currentRows, originalRow];
          }
        }
        return next;
      });

      // Show user-friendly error message
      const failedCount = failedIPs.length;
      setLinkedErrorMsg(`Delete failed for ${failedCount} device(s). They have been added back to the list.`);
    }
  } catch (err) {
    // Hard failure: roll back all to snapshot
    setAssocByIrc(prevAssoc);
    setLinkedErrorMsg(err?.message || 'Delete Associated systems failed');
  } finally {
    setDeletingLinked(false);
    setLinkedConfirmOpen(false);
    // Clear all selections after operation
    setSelectedLinkedByIrc({});
  }
};

  async function bulkDeleteIRCs(ips) {
    const res = await authFetch('/api/v1/irc', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ips }),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Delete failed: HTTP ${res.status} — ${text}`);
    }
    return res.json().catch(() => ({}));
  }

  const handleBulkDelete = async () => {
    const ips = Array.from(selectedIPs);
    if (ips.length === 0) return;

    setDeleting(true);
    setErrorMsg('');

    const prev = ircList;
    setIrcList(list => list.filter(x => !selectedIPs.has(x.ip)));

    try {
      const result = await bulkDeleteIRCs(ips);
      if (result?.failed && Object.keys(result.failed).length > 0) {
        const failedIPs = new Set(Object.keys(result.failed));
        setIrcList(list => [...list, ...prev.filter(x => failedIPs.has(x.ip))]);
        setErrorMsg(`Some deletions failed: ${JSON.stringify(result.failed)}`);
      }
      
    setAssocByIrc(prev => {
        const next = { ...prev };
        ips.forEach(ip => {
          if (!result.failed || !result.failed[ip]) {
            delete next[ip];
          }
        });
        return next;
      });
      
      setExpanded(prev => {
          const next = { ...prev };
          ips.forEach(ip => delete next[ip]);
          return next;
        });

    } catch (err) {
      setIrcList(prev);
      setErrorMsg(err.message || 'Bulk delete failed');
    } finally {
      setDeleting(false);
      setBulkConfirmOpen(false);
      setSelectedIPs(new Set());
    }
  };

  const onExpand = async (ircIp, ensureOpen = false) => {
    const nextState = ensureOpen ? true : !expanded[ircIp];
    setExpanded(prev => ({ ...prev, [ircIp]: nextState }));
    if (!nextState) return;

    try {
      const res = await authFetch(`/api/v1/irc/${ircIp}/racdevice`);
      if (!res.ok) return;
      const payload = await res.json();
      const items = Array.isArray(payload.data)
        ? payload.data.map(item => ({
            ip: item.ip ?? item.Ip ?? '',
            status: item.status || item.state || 'Unknown',
            type: (item.systemtypedesc || item.SystemTypeDesc || '').toUpperCase(),
            selectedType: item.type ?? null,
          }))
        : [];
      setAssocByIrc(prev => ({ ...prev, [ircIp]: items }));
    } catch (e) {
      console.error('Error fetching associated systems:', e);
    }
  };

  return (
  <div style={{ paddingBottom: 20 }}>
    {/* Top App Bar with Profile Icon & Menu */}
    <AppBar position="static" color="primary" elevation={1}>
      <Toolbar sx={{ display: 'flex', justifyContent: 'space-between' }}>
        <Typography variant="h6" sx={{ fontWeight: 600 }} color="#fff">
          IRC Dashboard
        </Typography>

        <IconButton
          size="large"
          edge="end"
          color="inherit"
          aria-label="account"
          aria-controls={profileMenuOpen ? 'profile-menu' : undefined}
          aria-haspopup="true"
          aria-expanded={profileMenuOpen ? 'true' : undefined}
          onClick={openProfileMenu}
        >
          <AccountCircle />
        </IconButton>

        <Menu
          id="profile-menu"
          anchorEl={profileAnchor}
          open={profileMenuOpen}
          onClose={closeProfileMenu}
          anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
          transformOrigin={{ vertical: 'top', horizontal: 'right' }}
          PaperProps={{ sx: { minWidth: 200 } }}
        >
          
        
          <MenuItem
            disableRipple
            onClick={(e) => e.stopPropagation()}     // prevent menu close on click
            sx={{ cursor: 'default', py: 1.5 }}
          >
            <Avatar sx={{ bgcolor: 'primary.main', width: 28, height: 28, mr: 1 }}>
              <AccountCircle />
            </Avatar>
            <Box>
              <Typography variant="body1" sx={{ fontWeight: 600, lineHeight: 1, color: 'text.primary' }}>
                admin
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Administrator
              </Typography>
            </Box>
          </MenuItem>

          <Divider />

          <MenuItem onClick={() => { closeProfileMenu(); setOpenChange(true); }}>
            <ManageAccountsIcon fontSize="small" style={{ marginRight: 8 }} />
            Change Password
          </MenuItem>
          <MenuItem onClick={async () => { 
            closeProfileMenu(); 
            await onLogout?.(); 
          }}>
            <LogoutIcon fontSize="small" style={{ marginRight: 8 }} />
            Logout
          </MenuItem>
        </Menu>
      </Toolbar>
    </AppBar>

    {/* === Your existing dashboard content below === */}
    <div style={{ padding: '20px' }}>
      {/* Keep your existing buttons, lists, dialogs, etc. */}
    </div>

   
{/* Change Password dialog (inside dashboard) */}
<Dialog open={openChange} onClose={closeChange}>
  <DialogTitle>Change Admin Password</DialogTitle>
  <DialogContent>
    <TextField
      type="password"
      label="Current Password"
      fullWidth
      margin="dense"
      value={oldPass}
      onChange={(e) => setOldPass(e.target.value)}
    />
    <TextField
      type="password"
      label="New Password"
      fullWidth
      margin="dense"
      value={newPass}
      onChange={(e) => setNewPass(e.target.value)}
      helperText="Minimum 4 characters"
    />
    <TextField
      type="password"
      label="Confirm New Password"
      fullWidth
      margin="dense"
      value={confirm}
      onChange={(e) => setConfirm(e.target.value)}
    />
    {cpError && <Typography color="error" sx={{ mt: 1 }}>{cpError}</Typography>}

    {/* Helper text */}
    <Typography variant="caption" sx={{ mt: 2, display: 'block', color: 'text.secondary' }}>
      Or reset credentials back to <strong>admin / admin</strong>.
    </Typography>
  </DialogContent>

  <DialogActions sx={{ justifyContent: 'space-between' }}>
    {/* Reset-to-default inside Change Password dialog: opens confirm dialog */}
    <Button
      color="error"
      variant="outlined"
      onClick={() => {
        setResetError('');
        setOpenResetConfirm(true);
      }}
    >
      Reset to default
    </Button>

    <Box>
      <Button onClick={closeChange} disabled={cpBusy} sx={{ mr: 1 }}>
        Cancel
      </Button>
      <Button onClick={async () => {
        setCpError('');
        if (!hasMinLen(newPass, 4)) {
          setCpError('Password must be at least 4 characters.');
          return;
        }
        if (newPass !== confirm) {
          setCpError('New password and confirmation do not match.');
          return;
        }
        setCpBusy(true);
        const res = await changePassword(oldPass, newPass);
        setCpBusy(false);
        if (!res.ok) {
          setCpError(res.message || 'Unable to change password.');
          return;
        }
        closeChange();
      }} variant="contained" disabled={cpBusy}>
        {cpBusy ? 'Saving…' : 'Save'}
      </Button>
    </Box>
  </DialogActions>
</Dialog>

{/* Confirm Reset dialog (appears after clicking "Reset to default") */}
<Dialog
  open={openResetConfirm}
  onClose={() => (resetBusy ? null : setOpenResetConfirm(false))}
>
  <DialogTitle>Confirm Reset</DialogTitle>
  <DialogContent>
    <Typography variant="body2">
      This will reset admin credentials to <strong>admin / admin</strong>.
    </Typography>
    <Typography variant="body2" sx={{ mt: 1 }}>
      You will be logged out and need to sign in again.
    </Typography>
    {resetError && <Typography color="error" sx={{ mt: 1 }}>{resetError}</Typography>}
  </DialogContent>
  <DialogActions>
    <Button onClick={() => setOpenResetConfirm(false)} disabled={resetBusy}>
      Cancel
    </Button>
    <Button
      onClick={async () => {
        setResetError('');
        setResetBusy(true);
        try {
          const res = await resetToDefault(); // resets passHash => "admin"; clears session
          if (!res.ok) throw new Error(res.message || 'Reset failed');

          // Clear local form fields
          setOldPass('');
          setNewPass('');
          setConfirm('');

          // Close both dialogs
          setOpenResetConfirm(false);
          closeChange();

          // Kick back to login
          onLogout?.();
        } catch (e) {
          setResetError(e.message || 'Reset failed');
        } finally {
          setResetBusy(false);
        }
      }}
      variant="contained"
      color="error"
      disabled={resetBusy}
    >
      {resetBusy ? 'Resetting…' : 'Confirm Reset'}
    </Button>
  </DialogActions>
</Dialog>


      <div style={{ marginBottom: '20px' }}>
        <Button variant="contained" color="primary" onClick={() => setOpenIRCDialog(true)}>ADD IRC</Button>
        <Button
          variant="outlined"
          color="error"
          onClick={() => setBulkConfirmOpen(true)}
          disabled={selectedIPs.size === 0}
          sx={{
            color: '#fff',
            '&.Mui-disabled': { color: '#fff', opacity: 0.5 },
          }}
        >
          Delete IRC
        </Button>

        <Button
          variant="outlined"
          color="error"
          onClick={() => setLinkedConfirmOpen(true)}
          disabled={!anyLinkedSelected || deletingLinked}
          sx={{
            ml: 1,
            color: '#fff',
            '&.Mui-disabled': { color: '#fff', opacity: 0.5 },
          }}
        >
          Delete Systems
        </Button>
      </div>




      <div style={{ marginTop: '20px' }}>
        {loading ? (
          <CircularProgress />
        ) : error ? (
          <Typography color="error">Error: {error}</Typography>
        ) : ircList.length === 0 ? (
          <Typography sx={{ whiteSpace: 'pre' }}>{'    '}No IRC systems found.</Typography>
        ) : (
          ircList.map((irc, index) => (
            <div key={index} className="irc-bar" style={{ border: '1px solid #ddd', marginBottom: '12px', borderRadius: '8px' }}>
              <div className="irc-header" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', padding: '12px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <IconButton onClick={() => onExpand(irc.ip)}>
                    <ChevronRightIcon
                      style={{
                        transform: expanded[irc.ip] ? 'rotate(90deg)' : 'rotate(0deg)',
                        transition: 'transform 0.2s',
                      }}
                    />
                  </IconButton>

                  <Checkbox
                    checked={selectedIPs.has(irc.ip)}
                    onChange={() => toggleSelect(irc.ip)}
                    inputProps={{ 'aria-label': `Select ${irc.ip}` }}
                    size="small"
                  />
                  <strong>{irc.ip}</strong>
                </div>

                <LiveStatus
                  ip={irc.ip}
                  initialState={irc.state}
                  initialValve1={irc.valve1state}
                  initialValve2={irc.valve2state}
                  liveStatus={liveStatusMap[irc.ip]}
                />

                <Tooltip title="Click here to add more associated systems">
                  <>
                    <Button
                      variant="outlined"
                      onClick={async () => { setSelectedIRC(irc.ip); await refreshIRCList(); setOpenSystemDialog(true); }}
                    >
                      Add Rack Device
                    </Button>
                    {/* <Button variant="outlined" color="secondary" onClick={() => setOpenScriptDialog(true)}>
                      Add Custom Action
                    </Button> */}
                  </>
                </Tooltip>
              </div>

              
            <IrcRow
              ircIp={irc.ip}
              expanded={!!expanded[irc.ip]}
              assocByIrc={assocByIrc}
              setAssocByIrc={setAssocByIrc}
              selectedLinkedIPs={selectedLinkedByIrc[irc.ip] ?? new Set()}    
              onToggleLinkedIp={(ip) => toggleLinkedIp(irc.ip, ip)}
              onToggleSelectAll={(checked) => toggleSelectAllLinked(irc.ip, checked)}
            />

            </div>
          ))
        )}
      </div>

      {/* Add IRC dialog */}
      <Dialog open={openIRCDialog} onClose={() => setOpenIRCDialog(false)}>
        <DialogTitle>Add IRC</DialogTitle>
        <DialogContent>
          <TextField label="IP" fullWidth margin="dense" value={newIRC.ip} onChange={(e) => setNewIRC({ ...newIRC, ip: e.target.value })} error={Boolean(ircErrors.ip)}helperText={ircErrors.ip}/>
          <TextField label="Username" fullWidth margin="dense" value={newIRC.username} onChange={(e) => setNewIRC({ ...newIRC, username: e.target.value })} error={Boolean(ircErrors.username)} helperText={ircErrors.username}/>
          <TextField label="Password" type="password" fullWidth margin="dense" value={newIRC.password} onChange={(e) => setNewIRC({ ...newIRC, password: e.target.value })} error={Boolean(ircErrors.password)} helperText={ircErrors.password}/>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenIRCDialog(false)}>Cancel</Button>
          
                  <Button
                    onClick={() => {
                      const nextErrors = { ip: '', username: '', password: '' };

                      if (!isValidIPv4(newIRC.ip)) nextErrors.ip = 'Please enter a valid IPv4 address.';
                      if (!isNonEmpty(newIRC.username)) nextErrors.username = 'Username is required.';
                      if (!hasMinLen(newIRC.password, 4)) nextErrors.password = 'Password must be at least 4 characters.';

                      setIrcErrors(nextErrors);

                      if (Object.values(nextErrors).some(msg => msg !== '')) return; // stop if errors exist

                      handleAddIRC();
                    }}
                  >
                    Add
                  </Button>

        </DialogActions>
      </Dialog>

     
<Dialog
  open={openSystemDialog}
  onClose={() => setOpenSystemDialog(false)}
  // disableEnforceFocus
  // disableAutoFocus
  // disableRestoreFocus
  ModalProps={{ keepMounted: false }}
>
  <DialogTitle>Add Rack Device</DialogTitle>
  <DialogContent>
    <TextField
      label="IP"
      fullWidth
      margin="dense"
      value={newSystem.ip}
      onChange={(e) => setNewSystem({ ...newSystem, ip: e.target.value })}
      error={Boolean(sysErrors.ip)}
      helperText={sysErrors.ip}

    />
    <TextField
      label="Username"
      fullWidth
      margin="dense"
      value={newSystem.username}
      onChange={(e) => setNewSystem({ ...newSystem, username: e.target.value })}
      error={Boolean(sysErrors.username)}
      helperText={sysErrors.username}

    />
    <TextField
      label="Password"
      type="password"
      fullWidth
      margin="dense"
      value={newSystem.password}
      onChange={(e) => setNewSystem({ ...newSystem, password: e.target.value })}
      error={Boolean(sysErrors.password)}
      helperText={sysErrors.password}

    />

    {/* Device Type Select — controlled menu open/close */}
    <Select
      fullWidth
      value={newSystem.systemtypedesc ?? ""}
      onChange={(e) => setNewSystem({ ...newSystem, systemtypedesc: e.target.value })}
      displayEmpty
      sx={{ mt: 2 }}
      open={typeOpen}
      onOpen={() => setTypeOpen(true)}
      onClose={() => setTypeOpen(false)}
      MenuProps={{
        disablePortal: true,
        onClose: () => setTypeOpen(false),
      }}
    >
      <MenuItem value="" disabled sx={{ color: '#666', fontWeight: 400 }}>
        {systemTypes.length ? 'Select Device Type' : 'Loading device types…'}
      </MenuItem>
      {systemTypes?.map((st) => (
        <MenuItem key={st} value={st}>
          {st.toUpperCase()}
        </MenuItem>
      ))}
    </Select>
    
    {Boolean(sysErrors.systemtypedesc) && (
      <Typography variant="caption" color="error" sx={{ mt: 0.5 }}>
        {sysErrors.systemtypedesc}
      </Typography>
    )}

    {/* Shutdown type (boolean) + timeout seconds shown only when graceful */}
    <Typography sx={{ mt: 2, mb: 1 }} variant="body1" fontWeight={500}>
      Shutdown type:
    </Typography>

    <RadioGroup
      name="shutdown-type"
      row
      value={
        newSystem.isforcefulshutdown === true ? 'forceful' :
        newSystem.isforcefulshutdown === false ? 'graceful' : ''
      }
      onChange={(e) => {
        const next = e.target.value;
        setNewSystem(prev => ({
          ...prev,
          isforcefulshutdown: next === 'forceful',
          timeout: next === 'graceful' ? (prev.timeout || '300') : '',
        }));
      }}
    >
      <FormControlLabel value="forceful" control={<Radio />} label="Forceful" />
      <FormControlLabel value="graceful" control={<Radio />} label="Graceful" />
    </RadioGroup>

    {newSystem.isforcefulshutdown === false && (
      <TextField
        label="Timeout (seconds)"
        fullWidth
        margin="dense"
        type="number"
        inputProps={{ min: 0, step: 5 }}
        value={newSystem.timeout || ''}
        onChange={(e) => setNewSystem(prev => ({ ...prev, timeout: e.target.value }))}
        placeholder="300"
        helperText="Default: 300 seconds"
      />
    )}
  </DialogContent>

  <DialogActions>
    <Button
      type="button"
      onClick={() => {
        setTypeOpen(false);                                  // close Select menu first
        if (document.activeElement instanceof HTMLElement) {
          document.activeElement.blur();                     // release focus
        }
        Promise.resolve().then(() => setOpenSystemDialog(false));                        // now close dialog
      }}
    >
      Cancel
    </Button>
   
      <Button
        type="button"
        onClick={async () => {
          const nextErrors = {
            ip: '',
            username: '',
            password: '',
            systemtypedesc: '',
            timeout: '',
          };

          if (!isValidIPv4(newSystem.ip)) nextErrors.ip = 'Please enter a valid IPv4 address.';
          if (!isNonEmpty(newSystem.username)) nextErrors.username = 'Username is required.';
          if (!hasMinLen(newSystem.password, 4)) nextErrors.password = 'Password must be at least 4 characters.';
          if (!isNonEmpty(newSystem.systemtypedesc)) nextErrors.systemtypedesc = 'Select a device type.';
          if (newSystem.isforcefulshutdown === false) {
            if (!isNonNegativeIntegerString(String(newSystem.timeout ?? ''))) {
              nextErrors.timeout = 'Timeout must be a non-negative integer (e.g., 300).';
            }
          }

          setSysErrors(nextErrors);

          if (Object.values(nextErrors).some(msg => msg !== '')) return; // stop if errors exist

          await handleAddSystem();
        }}
      >
        Add
      </Button>

  </DialogActions>
</Dialog>


      {/* Upload script dialog
      <Dialog open={openScriptDialog} onClose={() => setOpenScriptDialog(false)}>
        <DialogTitle>Upload Custom Script</DialogTitle>
        <DialogContent>
          <input
            type="file"
            accept=".sh,.py,.txt"
            onChange={(e) => setScriptFile(e.target.files[0])}
            style={{ marginTop: '10px' }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenScriptDialog(false)}>Cancel</Button>
          <Button
            variant="contained"
            color="primary"
            onClick={() => {
              if (scriptFile) {
                console.log('Uploading script:', scriptFile.name);
                // TODO: implement upload logic (POST to backend)
              }
              setOpenScriptDialog(false);
            }}
          >
            Submit
          </Button>
        </DialogActions>
      </Dialog> */}

      {/* Bulk delete confirm dialog */}
      <Dialog
        open={bulkConfirmOpen}
        onClose={() => (deleting ? null : setBulkConfirmOpen(false))}
        disableEscapeKeyDown={deleting}
        ModalProps={{ keepMounted: true }}
      >
        <DialogTitle>Delete Selected IRCs</DialogTitle>
        <DialogContent>
          {deleting ? (
            <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
              <CircularProgress size={20} />
              <span>Deleting…</span>
            </div>
          ) : (
            <div>
              Are you sure you want to delete:
              <ul style={{ marginTop: 8 }}>
                {Array.from(selectedIPs).map(ip => <li key={ip}>{ip}</li>)}
              </ul>
              {errorMsg && <div style={{ color: '#c62828', marginTop: 8 }}>{errorMsg}</div>}
            </div>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setBulkConfirmOpen(false)} disabled={deleting}>Cancel</Button>
          <Button onClick={handleBulkDelete} color="error" variant="contained" disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete'}
          </Button>
        </DialogActions>
      </Dialog>

      
     
 
    <Dialog open={linkedConfirmOpen} onClose={() => setLinkedConfirmOpen(false)}>
      <DialogTitle>Delete Associated systems</DialogTitle>
      <DialogContent>
        {linkedErrorMsg ? (
          <Typography color="error" sx={{ mb: 1 }}>
            {linkedErrorMsg}
          </Typography>
        ) : null}
        Are you sure you want to delete {getAllSelectedLinkedIPs().length} Assosciated system(s)?
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setLinkedConfirmOpen(false)} disabled={deletingLinked}>
          Cancel
        </Button>
        <Button
          color="error"
          variant="contained"
          onClick={handleDeleteLinkedSystems}
          disabled={deletingLinked}
        >
          {deletingLinked ? 'Deleting...' : 'Delete'}
        </Button>
      </DialogActions>
    </Dialog>



    </div>
  );
}
