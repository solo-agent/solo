/** Parse a GitHub issue URL into owner, repo, and issue number. */
export interface GitHubIssueInfo {
  owner: string;
  repo: string;
  number: number;
}

export function parseGitHubIssueURL(url: string): GitHubIssueInfo | null {
  // Match: https://github.com/owner/repo/issues/123
  const match = url.match(/github\.com\/([^/]+)\/([^/]+)\/issues\/(\d+)/);
  if (!match) return null;
  return {
    owner: match[1],
    repo: match[2],
    number: parseInt(match[3], 10),
  };
}

/** Format a GitHub issue reference for display. */
export function formatGitHubIssueRef(info: GitHubIssueInfo): string {
  return `[GH ${info.owner}/${info.repo}#${info.number}]`;
}
