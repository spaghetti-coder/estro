const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');
const express = require('express');
const { exec } = require('child_process');
const { promisify } = require('util');
const crypto  = require('crypto');
const session = require('express-session');
const bcrypt  = require('bcryptjs');
const rateLimit = require('express-rate-limit');

const execP = promisify(exec);
const CLIENT_TIMEOUT_BUFFER = 10000; // client AbortController fires after server timeout + this buffer
const REMEMBER_ME_MAX_AGE = 30 * 24 * 60 * 60 * 1000;
const JOB_TTL_MS = 10 * 60 * 1000;

const jobs = new Map();
const SSH_OPTS = '-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null';
const configFile = ['config.yaml', 'config.yml'].find(f => fs.existsSync(path.join(__dirname, f))) || 'config.yaml';

let cfg;
try {
  cfg = yaml.load(fs.readFileSync(path.join(__dirname, configFile), 'utf8'));
} catch (e) {
  console.error(`Failed to load ${configFile}:`, e.message);
  process.exit(1);
}

const globalCfg = cfg.global || {};
const { hostname = '127.0.0.1', port = 3000, secret: cfgSecret } = globalCfg;
const sessionSecret = cfgSecret || crypto.randomBytes(32).toString('hex');
const services = [];
for (const section of (cfg.sections || [])) {
  for (const svc of (section.services || [])) {
    services.push({
      ...svc,
      _section: section.title,
      _secTimeout: section.timeout,
      _secConfirm: section.confirm,
      _secAllowed: section.allowed,
      _secCollapsable: section.collapsable,
      _secRemote: section.remote,
      _secColumns: section.columns,
    });
  }
}
const users = cfg.users || {};

// --- Helpers ---

function shellEscape(cmd) {
  return cmd.replace(/'/g, "'\\''");
}

function validateHost(host) {
  if (!/^[a-zA-Z0-9._@:/-]+$/.test(host)) throw new Error(`Invalid remote host: ${host}`);
}

function buildCmd(command, remote) {
  const cmd = Array.isArray(command) ? command.join(' && ') : command;
  if (!remote) return cmd;
  const hosts = Array.isArray(remote) ? remote : [remote];
  if (hosts.length === 0) return cmd;
  hosts.forEach(validateHost);
  // reduceRight wraps cmd in nested ssh calls: last host is outermost shell,
  // so execution hops through the chain left-to-right as listed in config.
  return hosts.reduceRight((innerCmd, host) => {
    return `ssh ${SSH_OPTS} ${host} '${shellEscape(innerCmd)}'`;
  }, cmd);
}

function secKey(field) {
  return '_sec' + field.charAt(0).toUpperCase() + field.slice(1);
}

function cascadeField(svc, field, builtinDefault) {
  if (svc[field] !== undefined) return svc[field];
  const sk = secKey(field);
  if (svc[sk] !== undefined) return svc[sk];
  if (globalCfg[field] !== undefined) return globalCfg[field];
  return builtinDefault;
}

function getSvcTimeout(svc) {
  return cascadeField(svc, 'timeout', 60) * 1000;
}

function resolveUsers(svc) {
  const allowed = cascadeField(svc, 'allowed', null);
  // null/[] both mean "no restriction" (public). Non-empty array = explicit allowlist.
  if (allowed === null || allowed.length === 0) return null;
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

function isServiceAccessible(svc, username) {
  const allowed = resolveUsers(svc);
  return allowed === null || (!!username && allowed.includes(username));
}

function logStream(label, content, fn = console.log) {
  if (content) {
    fn(`~~~~~ ${label} START ~~~~~`);
    fn(content.trim());
    fn(`~~~~~ ${label} END ~~~~~`);
  }
}

function serializeService(svc, i, username) {
  const allowedUsers = resolveUsers(svc);
  const isPublic = allowedUsers === null;
  return {
    id: i,
    title: svc.title,
    timeout: getSvcTimeout(svc) + CLIENT_TIMEOUT_BUFFER,
    confirm: cascadeField(svc, 'confirm', true),
    section: svc._section || null,
    sectionCollapsable: cascadeField(svc, 'collapsable', true),
    sectionColumns: cascadeField(svc, 'columns', 3),
    public: isPublic,
    accessible: isPublic || (!!username && allowedUsers.includes(username)),
    allowedUsers,
  };
}

// --- Route handlers ---

function listServices(req, res) {
  const username = req.session?.user || null;
  res.json(services.map((svc, i) => serializeService(svc, i, username)));
}

async function runService(req, res) {
  const entry = services[parseInt(req.params.svc, 10)];
  if (!entry) return res.status(404).json({ error: 'Unknown service' });

  if (!isServiceAccessible(entry, req.session?.user)) return res.status(403).json({ error: 'Forbidden' });

  const remote = cascadeField(entry, 'remote', null);
  let cmd;
  try {
    cmd = buildCmd(entry.command, remote);
  } catch (e) {
    return res.status(400).json({ error: e.message });
  }

  const jobId = crypto.randomUUID();
  jobs.set(jobId, { status: 'running', title: entry.title });
  res.status(202).json({ jobId });

  (async () => {
    try {
      console.log(`Running ${entry.title}: ${cmd}`);
      const { stdout, stderr } = await execP(cmd, { timeout: getSvcTimeout(entry) });
      logStream('STDOUT', stdout);
      logStream('STDERR', stderr, console.warn);
      jobs.set(jobId, { status: 'done', title: entry.title, stdout, stderr });
    } catch (error) {
      console.error(`Command error for ${entry.title}:`, error.stderr || error.message);
      jobs.set(jobId, { status: 'error', title: entry.title, stdout: error.stdout || '', stderr: error.stderr || error.message });
    } finally {
      setTimeout(() => jobs.delete(jobId), JOB_TTL_MS);
    }
  })();
}

function getJob(req, res) {
  const job = jobs.get(req.params.id);
  if (!job) return res.status(404).json({ error: 'Unknown job' });
  res.json(job);
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
  if (rememberMe) req.session.cookie.maxAge = REMEMBER_ME_MAX_AGE;
  res.json({ username });
}

function logout(req, res) {
  req.session.destroy(() => { res.clearCookie('connect.sid'); res.status(204).end(); });
}

function getConfig(_req, res) {
  res.json({
    title: globalCfg.title || 'Estro',
    subtitle: globalCfg.subtitle ?? '',
    users: Object.keys(users),
  });
}

// --- Init ---

function init() {
  const app = express();
  app.use((req, res, next) => {
    res.setHeader('Content-Security-Policy', "default-src 'self'");
    next();
  });
  app.use(express.static(path.join(__dirname, 'public')));
  app.use(express.json());
  app.use(session({
    secret: sessionSecret,
    resave: false,
    saveUninitialized: false,
    cookie: { httpOnly: true, sameSite: 'strict' },
  }));
  app.use((req, res, next) => {
    console.log(`${new Date().toISOString()} ${req.method} ${req.url}`);
    next();
  });

  const loginLimiter = rateLimit({ windowMs: 15 * 60 * 1000, max: 10, standardHeaders: true, legacyHeaders: false });

  app.get('/config',    getConfig);
  app.get('/services',  listServices);
  app.get('/me',        getMe);
  app.post('/login',    loginLimiter, login);
  app.post('/logout',   logout);
  app.post('/run/:svc', runService);
  app.get('/jobs/:id',  getJob);

  app.listen(port, hostname, () => console.log(`Estro listening on http://${hostname}:${port}`));
}

init();
