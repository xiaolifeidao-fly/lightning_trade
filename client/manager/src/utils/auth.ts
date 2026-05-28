"use client";

const AUTH_TOKEN_KEY = "phoenix_manager_token";

function canUseBrowserStorage() {
  return typeof window !== "undefined";
}

export function getAuthToken() {
  if (!canUseBrowserStorage()) {
    return "";
  }
  return window.localStorage.getItem(AUTH_TOKEN_KEY) || window.sessionStorage.getItem(AUTH_TOKEN_KEY) || "";
}

export function setAuthToken(token: string, remember = true) {
  if (!canUseBrowserStorage()) {
    return;
  }
  clearAuthToken();
  const storage = remember ? window.localStorage : window.sessionStorage;
  storage.setItem(AUTH_TOKEN_KEY, token);
}

export function clearAuthToken() {
  if (!canUseBrowserStorage()) {
    return;
  }
  window.localStorage.removeItem(AUTH_TOKEN_KEY);
  window.sessionStorage.removeItem(AUTH_TOKEN_KEY);
}

export function isAuthenticated() {
  return getAuthToken().trim().length > 0;
}
