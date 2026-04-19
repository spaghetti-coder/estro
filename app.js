const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');
const express = require('express');
const { exec } = require('child_process');
const { promisify } = require('util');
const crypto  = require('crypto');
const session = require('express-session');
const bcrypt  = require('bcryptjs');

const execP = promisify(exec);
const CLIENT_TIMEOUT_BUFFER = 10000; // client AbortController fires after server timeout + this buffer
const SSH_OPTS = '-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null';
const configFile = ['config.yaml', 'config.yml'].find(f => fs.existsSync(path.join(__dirname, f))) || 'config.yaml';
const cfg = yaml.load(fs.readFileSync(path.join(__dirname, configFile), 'utf8'));
const { hostname = '127.0.0.1', port = 3000, timeout: timeoutCfg = 60, secret: cfgSecret } = cfg.config || {};
const timeout = timeoutCfg * 1000;
const sessionSecret = cfgSecret || crypto.randomBytes(32).toString('hex');
const services = [];
for (const section of (cfg.sections || [])) {
  for (const svc of (section.services || [])) {
    services.push({ ...svc, _section: section.title, _sectionAllowed: section.allowed || null, _sectionExpanded: section.expanded || false });
  }
}
const users = cfg.users || {};

// --- Helpers ---

function shellEscape(cmd) {
  return cmd.replace(/'/g, "'\\''");
}

function buildCmd(command, remote) {
  const cmd = Array.isArray(command) ? command.join(' && ') : command;
  if (!remote) return cmd;
  return `ssh ${SSH_OPTS} ${remote} '` + shellEscape(cmd) + "'";
}

function getSvcTimeout(svc) {
  return svc.timeout != null ? svc.timeout * 1000 : timeout;
}

function resolveUsers(svc) {
  const allowed = svc.allowed?.length ? svc.allowed : (svc._sectionAllowed ?? null);
  if (!allowed) return null;
  const result = new Set();
  for (const name of allowed) {
    if (users[name]) {
      result.add(name);
    } else {
      for (const [uname, u] of Object.entries(users)) {
        if ((u.groups || []).includes(name)) result.add(uname);
      }
    }
  }
  return [...result];
}

// --- Route handlers ---

function listServices(req, res) {
  const username = req.session?.user || null;
  const list = services.map((svc, i) => {
    const allowedUsers = resolveUsers(svc);
    const isPublic = allowedUsers === null;
    return {
      id: i, title: svc.title, timeout: getSvcTimeout(svc) + CLIENT_TIMEOUT_BUFFER,
      confirm: svc.confirm !== false,
      section: svc._section || null,
      sectionExpanded: svc._sectionExpanded || false,
      public: isPublic,
      accessible: isPublic || (!!username && allowedUsers.includes(username)),
      allowedUsers,
    };
  });
  res.json(list);
}

async function runService(req, res) {
  const entry = services[parseInt(req.params.svc, 10)];
  if (!entry) return res.status(404).send('Unknown service');

  const allowed = resolveUsers(entry);
  if (allowed !== null && !allowed.includes(req.session?.user)) return res.status(403).send('Forbidden');

  const cmd = buildCmd(entry.command, entry.remote);
  try {
    console.log(`Running ${entry.title}: ${cmd}`);
    const { stdout, stderr } = await execP(cmd, { timeout: getSvcTimeout(entry) });
    if (stdout) {
      console.log('~~~~~ STDOUT START ~~~~~');
      console.log(stdout.trim());
      console.log('~~~~~ STDOUT END ~~~~~');
    }
    if (stderr) {
      console.warn('~~~~~ STDERR START ~~~~~');
      console.warn(stderr.trim());
      console.warn('~~~~~ STDERR END ~~~~~');
    }
    return res.send(`${entry.title} done`);
  } catch (error) {
    console.error(`Error running ${entry.title}:`, error);
    return res.status(500).send(`Error: ${error.stderr || error.message}`);
  }
}

function getMe(req, res) {
  const username = req.session?.user || null;
  if (!username) return res.json(null);
  res.json({ username, groups: users[username]?.groups || [] });
}

async function login(req, res) {
  const { username, password, rememberMe } = req.body || {};
  if (!username || !password) return res.status(400).json({ error: 'Username and password required' });
  const u = users[username];
  if (!u || !(await bcrypt.compare(password, u.password)))
    return res.status(401).json({ error: 'Invalid username or password' });
  req.session.user = username;
  if (rememberMe) req.session.cookie.maxAge = 30 * 24 * 60 * 60 * 1000;
  res.json({ username });
}

function logout(req, res) {
  req.session.destroy(() => { res.clearCookie('connect.sid'); res.status(204).end(); });
}

// --- Init ---

function init() {
  const app = express();
  app.use(express.static(path.join(__dirname, 'public')));
  app.use(express.json());
  app.use(session({
    secret: sessionSecret,
    resave: false,
    saveUninitialized: false,
    cookie: { httpOnly: true, sameSite: 'lax' },
  }));
  app.use((req, res, next) => {
    console.log(`${new Date().toISOString()} ${req.method} ${req.url}`);
    next();
  });

  app.get('/config',    (_, res) => res.json({ title: cfg.config.title || 'Estro', subtitle: cfg.config.subtitle ?? '', users: Object.keys(users) }));
  app.get('/services',  listServices);
  app.get('/me',        getMe);
  app.post('/login',    login);
  app.post('/logout',   logout);
  app.post('/run/:svc', runService);

  app.listen(port, hostname, () => console.log(`Estro listening on http://${hostname}:${port}`));
}

init();
