
// src/LoginPage.jsx
import { useEffect, useState } from 'react';
import {
  Box, Paper, TextField, Button, Typography, InputAdornment, IconButton
} from '@mui/material';
import Visibility from '@mui/icons-material/Visibility';
import VisibilityOff from '@mui/icons-material/VisibilityOff';
import LockIcon from '@mui/icons-material/Lock';
import { ADMIN_USERNAME, isAuthenticated, login, logout } from './auth';

export default function LoginPage({ onLoggedIn }) {
  const [username, setUsername] = useState(ADMIN_USERNAME);
  const [password, setPassword] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [hint] = useState('Sign in with default admin credentials.');

  useEffect(() => {
    (async () => {
      let hasSession = false;
      try { hasSession = sessionStorage.getItem('irc_logged_in') === '1'; } catch {}
      if (!hasSession) {
        try { await logout(); } catch {}
        return;
      }
      const ok = await isAuthenticated();
      if (ok) onLoggedIn?.();
    })();
  }, [onLoggedIn]);

  const onSubmit = async (e) => {
    e.preventDefault();
    setError('');
    setBusy(true);
    const res = await login(username, password);
    setBusy(false);
    if (res.ok) {
      try { sessionStorage.setItem('irc_logged_in', '1'); } catch {}
      onLoggedIn?.();
    }
    else setError(res.message || 'Invalid username or password.');
  };

  return (
    <Box sx={{
      minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'linear-gradient(135deg, #e3f2fd, #f3e5f5)'
    }}>
      <Paper elevation={3} sx={{ p: 4, width: '100%', maxWidth: 460 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <LockIcon color="primary" />
          <Typography variant="h5">IRC Dashboard</Typography>
        </Box>

        <form onSubmit={onSubmit}>
          <TextField
            fullWidth
            label="Username"
            margin="normal"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoFocus
          />

          <TextField
            fullWidth
            label="Password"
            margin="normal"
            type={showPass ? 'text' : 'password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={() => setShowPass(s => !s)} edge="end" aria-label="toggle password visibility">
                    {showPass ? <VisibilityOff /> : <Visibility />}
                  </IconButton>
                </InputAdornment>
              )
            }}
          />

          {error && <Typography color="error" sx={{ mt: 1 }}>{error}</Typography>}

          <Button
            type="submit"
            variant="contained"
            color="primary"
            fullWidth
            sx={{ mt: 2 }}
            disabled={busy}
          >
            {busy ? 'Signing in…' : 'Login'}
          </Button>

          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 2 }}>
            <Typography variant="caption" sx={{ color: 'text.secondary' }}>
              {hint}
            </Typography>
          </Box>
        </form>
      </Paper>
    </Box>
  );
}
