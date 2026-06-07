import DOMPurify from 'dompurify';

/**
 * Sanitize HTML string before passing to dangerouslySetInnerHTML.
 *
 * Allows formatting tags (mark, span, b, strong, i, em, a, br, p),
 * code blocks (pre, code, div), and inline styles (needed by shiki syntax
 * highlighting). All other tags and attributes are removed.
 */
export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      'mark',
      'span',
      'code',
      'pre',
      'div',
      'b',
      'strong',
      'i',
      'em',
      'a',
      'br',
      'p',
    ],
    ALLOWED_ATTR: ['class', 'href', 'target', 'rel', 'style'],
  });
}
