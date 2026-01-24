export function setToLS(key: string, value: string): void {
  window.localStorage.setItem(key, value);
}

export function getFromLS(key: string): string | null {
  const value = window.localStorage.getItem(key);

  if (value) {
    return value;
  }

  return null;
}
