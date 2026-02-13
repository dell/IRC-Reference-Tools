
// src/App.js
import React, { useEffect, useState } from 'react';
import { CircularProgress } from '@mui/material';
import LoginPage from './LoginPage';
import IrcDashboard from './IrcDashboard';
import { isAuthenticated, logout, setUnauthorizedHandler } from './auth';

function App() {
  const [authReady, setAuthReady] = useState(false);
  const [loggedIn, setLoggedIn] = useState(false);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      try { sessionStorage.removeItem('irc_logged_in'); } catch {}
      setLoggedIn(false);
    });

    (async () => {
      let hasSession = false;
      try { hasSession = sessionStorage.getItem('irc_logged_in') === '1'; } catch {}

      if (!hasSession) {
        try { await logout(); } catch {}
        setLoggedIn(false);
        setAuthReady(true);
        return;
      }

      const ok = await isAuthenticated();
      if (!ok) {
        try { sessionStorage.removeItem('irc_logged_in'); } catch {}
      }
      setLoggedIn(ok);
      setAuthReady(true);
    })();

    return () => {
      setUnauthorizedHandler(null);
    };
  }, []);

  if (!authReady) {
    return <div style={{ padding: 20 }}><CircularProgress /></div>;
  }

  if (!loggedIn) {
    return <LoginPage onLoggedIn={() => {
      try { sessionStorage.setItem('irc_logged_in', '1'); } catch {}
      setLoggedIn(true);
    }} />;
  }

  const handleLogout = async () => {
    await logout();
    try { sessionStorage.removeItem('irc_logged_in'); } catch {}
    setLoggedIn(false);
  };

  return <IrcDashboard onLogout={handleLogout} />;
}

export default App;
