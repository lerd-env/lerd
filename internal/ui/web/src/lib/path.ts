// Replace a leading home directory with ~ for display, so path labels stay
// short without shipping the absolute path in the visible text. The full path
// is still used for actions like opening the folder.
export function homeShorten(path: string, home: string): string {
  if (!path || !home) return path;
  if (path === home) return '~';
  if (path.startsWith(home + '/')) return '~' + path.slice(home.length);
  return path;
}
