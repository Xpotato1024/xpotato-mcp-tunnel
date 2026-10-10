const marker = 'xpotato: startup Chrome check/download/cache cleanup suppressed.';
const pattern = /^([ \t]*)\/\/ Preemptively check\/download Chrome for PDF generation \(runs in background\)\r?\n\1ensureChromeAvailable\(\);/gm;
const compatError = () => {
  const message = 'Commander の起動時 Chrome 呼び出しが既知の形と一致しません。互換性レビューが必要です。';
  return Object.assign(new Error(message), { safeMessage: message });
};

export function suppressStartupChrome(source) {
  // Match the one reviewed v0.2.52 startup call, leaving PDF tool behavior intact.
  if ([...source.matchAll(pattern)].length !== 1) throw compatError();
  return source.replace(pattern, `$1// ${marker}`);
}

export function assertStartupChromeSuppressed(source) {
  if (source.split(marker).length !== 2 || [...source.matchAll(pattern)].length) throw compatError();
}
