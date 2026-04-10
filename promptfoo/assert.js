function normalizeTerms(terms) {
  if (Array.isArray(terms)) {
    return terms;
  }
  if (typeof terms === 'string') {
    return terms
      .split(',')
      .map((term) => term.trim())
      .filter(Boolean);
  }
  if (terms && typeof terms === 'object') {
    return Object.values(terms).map((term) => String(term));
  }
  return [];
}

function includesAllTerms(output, terms) {
  const normalized = String(output || '').toLowerCase();
  return normalizeTerms(terms).every((term) => normalized.includes(String(term).toLowerCase()));
}

function includesAnyStructure(output) {
  const normalized = String(output || '').toLowerCase();
  return ['<role>', '<instructions>', '<constraints>', '<format>', '<verification>', 'output format', 'checklist', 'before finishing'].some((term) =>
    normalized.includes(term),
  );
}

module.exports = {
  validatePromptImproverOutput: (output, context) => {
    const prompt = String(context?.vars?.prompt || '').trim();
    const action = String(context?.vars?.action || '');
    const terms = normalizeTerms(context?.vars?.required_terms);
    const text = String(output || '').trim();

    const checks = [];
    checks.push({
      pass: text.length > 0,
      score: text.length > 0 ? 1 : 0,
      reason: 'Output must be non-empty',
    });
    checks.push({
      pass: text.toLowerCase() !== prompt.toLowerCase(),
      score: text.toLowerCase() !== prompt.toLowerCase() ? 1 : 0,
      reason: 'Output should differ from the original prompt',
    });
    checks.push({
      pass: text.length >= prompt.length,
      score: text.length >= prompt.length ? 1 : 0,
      reason: 'Output should preserve or expand the original prompt',
    });
    if (terms.length > 0) {
      checks.push({
        pass: includesAllTerms(text, terms),
        score: includesAllTerms(text, terms) ? 1 : 0,
        reason: `Output should retain required terms: ${terms.join(', ')}`,
      });
    }
    if (action === 'enhance_local') {
      checks.push({
        pass: includesAnyStructure(text),
        score: includesAnyStructure(text) ? 1 : 0,
        reason: 'Local enhancement should add explicit structure or output expectations',
      });
    }

    const pass = checks.every((check) => check.pass);
    const score = checks.reduce((sum, check) => sum + check.score, 0) / checks.length;
    return {
      pass,
      score,
      reason: pass ? 'Prompt improver output looks stronger' : 'Prompt improver regression detected',
      componentResults: checks,
    };
  },
};
