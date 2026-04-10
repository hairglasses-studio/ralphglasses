const { spawnSync } = require('node:child_process');
const crypto = require('node:crypto');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');

function ensureBuiltBinary(repoRoot, name, pkgPath) {
  const cacheRoot = path.join(os.tmpdir(), 'hg-promptfoo-bins');
  const repoHash = crypto.createHash('sha1').update(repoRoot).digest('hex').slice(0, 12);
  const binDir = path.join(cacheRoot, repoHash);
  const binPath = path.join(binDir, `${name}${process.platform === 'win32' ? '.exe' : ''}`);
  if (fs.existsSync(binPath)) {
    return binPath;
  }

  fs.mkdirSync(binDir, { recursive: true });
  const build = spawnSync('go', ['build', '-o', binPath, pkgPath], {
    cwd: repoRoot,
    env: { ...process.env, GOWORK: 'off' },
    encoding: 'utf8',
    maxBuffer: 10 * 1024 * 1024,
  });
  if (build.status !== 0) {
    throw new Error((build.stderr || build.stdout || `go build failed with exit ${build.status}`).trim());
  }
  return binPath;
}

function resolveTimeoutMs() {
  const raw = process.env.PROMPTFOO_PROVIDER_TIMEOUT_MS || '90000';
  const parsed = Number.parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 90000;
}

class RalphglassesPromptImproverProvider {
  constructor(options) {
    this.providerId = options.id || 'ralphglasses-prompt-improver';
    this.config = options.config || {};
    this.repoRoot = path.resolve(__dirname, '..');
    this.binaryPath = ensureBuiltBinary(this.repoRoot, 'ralphglasses-prompt-improver', './cmd/prompt-improver');
  }

  id() {
    return this.providerId;
  }

  async callApi(prompt, context) {
    const vars = context?.vars || {};
    const action = vars.action || 'enhance_local';
    const taskType = vars.task_type || 'general';
    const env = {
      ...process.env,
      OLLAMA_BASE_URL: process.env.OLLAMA_BASE_URL || 'http://127.0.0.1:11434',
      OLLAMA_API_KEY: process.env.OLLAMA_API_KEY || 'ollama',
    };

    let args;
    switch (action) {
      case 'enhance_local':
        args = ['enhance', '--quiet', '--type', taskType, prompt];
        break;
      case 'improve_ollama':
        env.PROMPT_IMPROVER_LLM = '1';
        env.PROMPT_IMPROVER_PROVIDER = 'claude';
        // Keep the regression lane on the fast local alias unless explicitly overridden.
        env.PROMPT_IMPROVER_MODEL = process.env.OLLAMA_FAST_MODEL || 'code-fast';
        env.PROMPT_IMPROVER_BASE_URL = env.OLLAMA_BASE_URL;
        env.PROMPT_IMPROVER_API_KEY_ENV = 'OLLAMA_API_KEY';
        args = ['improve', '--quiet', '--provider', 'claude', '--type', taskType, prompt];
        break;
      default:
        throw new Error(`unsupported action ${action}`);
    }

    const result = spawnSync(this.binaryPath, args, {
      cwd: this.repoRoot,
      env,
      encoding: 'utf8',
      maxBuffer: 10 * 1024 * 1024,
      timeout: resolveTimeoutMs(),
    });

    if (result.error) {
      throw result.error;
    }
    if (result.status !== 0) {
      throw new Error((result.stderr || result.stdout || `command failed with exit ${result.status}`).trim());
    }

    return {
      output: (result.stdout || '').trim(),
    };
  }
}

module.exports = RalphglassesPromptImproverProvider;
