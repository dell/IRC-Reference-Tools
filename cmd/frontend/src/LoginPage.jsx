
// src/LoginPage.jsx
import { useEffect, useState } from 'react';
import {
  Box, Paper, TextField, Button, Typography, InputAdornment, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions
} from '@mui/material';
import Visibility from '@mui/icons-material/Visibility';
import VisibilityOff from '@mui/icons-material/VisibilityOff';
import LockIcon from '@mui/icons-material/Lock';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import { ADMIN_USERNAME, isAuthenticated, login, logout, changePassword } from './auth';

export default function LoginPage({ onLoggedIn }) {
  const [username, setUsername] = useState(ADMIN_USERNAME);
  const [password, setPassword] = useState('');
  const [showPass, setShowPass] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [hint] = useState('Sign in with default admin credentials.');

  // --- Default-password prompt state ---
  const [showDefaultPrompt, setShowDefaultPrompt] = useState(false);
  const [newPass, setNewPass] = useState('');
  const [confirmPass, setConfirmPass] = useState('');
  const [showNewPass, setShowNewPass] = useState(false);
  const [showConfirmPass, setShowConfirmPass] = useState(false);
  const [changeBusy, setChangeBusy] = useState(false);
  const [changeError, setChangeError] = useState('');

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
      if (res.defaultCredentials) {
        setShowDefaultPrompt(true);
      } else {
        onLoggedIn?.();
      }
    }
    else setError(res.message || 'Invalid username or password.');
  };

  const handleChangePassword = async () => {
    setChangeError('');
    if (!newPass.trim()) { setChangeError('New password cannot be empty.'); return; }
    if (newPass.length < 4) { setChangeError('Password must be at least 4 characters.'); return; }
    if (newPass !== confirmPass) { setChangeError('Passwords do not match.'); return; }

    setChangeBusy(true);
    const res = await changePassword(password, newPass);
    setChangeBusy(false);
    if (res.ok) {
      setShowDefaultPrompt(false);
      onLoggedIn?.();
    } else {
      setChangeError(res.message || 'Failed to change password.');
    }
  };

  const handleKeepDefault = () => {
    setShowDefaultPrompt(false);
    onLoggedIn?.();
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

      {/* Default-password change prompt dialog */}
      <Dialog open={showDefaultPrompt} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Change Default Password
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ mb: 2 }}>
            You are using the default credentials. It is recommended to change
            your password for better security.
          </Typography>

          <TextField
            fullWidth
            label="New Password"
            margin="normal"
            type={showNewPass ? 'text' : 'password'}
            value={newPass}
            onChange={(e) => setNewPass(e.target.value)}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={() => setShowNewPass(s => !s)} edge="end" size="small">
                    {showNewPass ? <VisibilityOff /> : <Visibility />}
                  </IconButton>
                </InputAdornment>
              )
            }}
          />

          <TextField
            fullWidth
            label="Confirm New Password"
            margin="normal"
            type={showConfirmPass ? 'text' : 'password'}
            value={confirmPass}
            onChange={(e) => setConfirmPass(e.target.value)}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <IconButton onClick={() => setShowConfirmPass(s => !s)} edge="end" size="small">
                    {showConfirmPass ? <VisibilityOff /> : <Visibility />}
                  </IconButton>
                </InputAdornment>
              )
            }}
          />

          {changeError && (
            <Typography color="error" sx={{ mt: 1 }}>{changeError}</Typography>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button
            onClick={handleKeepDefault}
            disabled={changeBusy}
          >
            Keep Default
          </Button>
          <Button
            variant="contained"
            onClick={handleChangePassword}
            disabled={changeBusy}
          >
            {changeBusy ? 'Changing…' : 'Change Password'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
