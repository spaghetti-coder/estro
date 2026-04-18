require('dotenv').config(); // load .env first

const express = require('express');
const { exec } = require('child_process');
const { promisify } = require('util');
const path = require('path');

const execP = promisify(exec);

const PORT = process.env.PORT || 5005;
// Directory containing the Ollama docker‑compose files (can be overridden via .env)
const OLLAMA_DIR = process.env.OLLAMA_DIR || '/home/varlog/docker/ollama';

// Map of services → command + optional exec options
const cmdMap = {
  openclaw: {
    cmd: '/usr/bin/env openclaw gateway restart && /usr/bin/env openclaw doctor --fix --non-interactive',
  },
  ollama: {
    cmd: 'docker compose down ollama && sleep 1 && docker compose up ollama -d',
    options: { cwd: OLLAMA_DIR },
  },
};

const app = express();

// Serve static UI
app.use(express.static(path.join(__dirname, 'static')));

// Lightweight request logging for visibility
app.use((req, res, next) => {
  console.log(`${new Date().toISOString()} ${req.method} ${req.url}`);
  next();
});

// Restart endpoint (keeps same behavior, implemented with async/await)
app.post('/restart/:svc', async (req, res) => {
  const { svc } = req.params;
  const entry = cmdMap[svc];
  if (!entry) {
    return res.status(404).send('Unknown service');
  }

  const { cmd, options = {} } = entry;
  try {
    console.log(`Executing restart for ${svc}: ${cmd}`);
    const { stdout, stderr } = await execP(cmd, { ...options, timeout: 60000 });
    if (stdout) console.log(`stdout: ${stdout.trim()}`);
    if (stderr) console.warn(`stderr: ${stderr.trim()}`);
    return res.send(`${svc} restarted`);
  } catch (error) {
    console.error(`Error restarting ${svc}:`, error);
    // preserve previous response shape (include stderr if present)
    return res.status(500).send(`Error: ${error.stderr || error.message}`);
  }
});

app.listen(PORT, '0.0.0.0', () => {
  console.log(`Restarter listening on http://0.0.0.0:${PORT}`);
});

// Export app for testing or external composition
module.exports = app;
