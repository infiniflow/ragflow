import DOMPurify from 'dompurify';

describe('rerun modal content sanitization', () => {
  it('strips unsafe html from interpolated pipeline step names', () => {
    const step = '<img src=x onerror="alert(1)"><script>alert(1)</script>';
    const html = `You are about to rerun the process starting from the <span class="text-text-secondary">${step}</span> step.`;
    const sanitized = DOMPurify.sanitize(html);

    expect(sanitized).not.toMatch(/onerror/i);
    expect(sanitized).not.toContain('<script');
    expect(sanitized).toContain('img');
  });
});
