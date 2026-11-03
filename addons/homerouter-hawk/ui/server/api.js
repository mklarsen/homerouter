import express from 'express';
import sqlite3 from 'sqlite3';
import basicAuth from 'express-basic-auth';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const app = express();
const PORT = process.env.PORT || 3000;

const DB_PATH = process.env.DATABASE_PATH || '/data/ip-tracker.db';
const AUTH_USER = process.env.HAWK_USER || 'admin';
const AUTH_PASS = process.env.HAWK_PASSWORD || 'admin123';

// Middleware
app.use(express.json());

// Basic auth middleware
const basicAuthMiddleware = basicAuth({
  users: { [AUTH_USER]: AUTH_PASS },
  challenge: true,
  realm: 'homerouter-hawk'
});

// Health check (no auth)
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// API routes (with auth)
app.get('/api/ip-history', basicAuthMiddleware, (req, res) => {
  const db = new sqlite3.Database(DB_PATH, sqlite3.OPEN_READONLY);
  const limit = Math.min(parseInt(req.query.limit || '1000'), 10000);
  
  db.all(
    `SELECT id, timestamp, ipv4, ipv6, change FROM ip_history ORDER BY timestamp DESC LIMIT ?`,
    [limit],
    (err, rows) => {
      db.close();
      if (err) {
        res.status(500).json({ error: err.message });
      } else {
        res.json(rows.reverse()); // Return chronological order
      }
    }
  );
});

app.get('/api/ip-changes', basicAuthMiddleware, (req, res) => {
  const db = new sqlite3.Database(DB_PATH, sqlite3.OPEN_READONLY);
  
  db.all(
    `SELECT id, timestamp, ipv4, ipv6 FROM ip_history WHERE change = 1 ORDER BY timestamp DESC LIMIT 100`,
    (err, rows) => {
      db.close();
      if (err) {
        res.status(500).json({ error: err.message });
      } else {
        res.json(rows.reverse());
      }
    }
  );
});

app.get('/api/latest', basicAuthMiddleware, (req, res) => {
  const db = new sqlite3.Database(DB_PATH, sqlite3.OPEN_READONLY);
  
  db.get(
    `SELECT timestamp, ipv4, ipv6 FROM ip_history ORDER BY id DESC LIMIT 1`,
    (err, row) => {
      db.close();
      if (err) {
        res.status(500).json({ error: err.message });
      } else {
        res.json(row || { ipv4: 'unknown', ipv6: 'unknown' });
      }
    }
  );
});

// Serve static React build
app.use(express.static(path.join(__dirname, '../dist')));

// SPA fallback
app.get('*', (req, res) => {
  res.sendFile(path.join(__dirname, '../dist/index.html'));
});

app.listen(PORT, '0.0.0.0', () => {
  console.log(`homerouter-hawk-ui listening on ${PORT}`);
  console.log(`Database: ${DB_PATH}`);
  console.log(`Auth: ${AUTH_USER}:${AUTH_PASS}`);
});
