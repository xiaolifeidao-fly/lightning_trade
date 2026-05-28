"use client";

import { useEffect, useState } from "react";
import {
  createUser,
  deleteUser,
  fetchUserDetail,
  fetchUserStats,
  fetchUsers,
  type UserListQuery,
  type UserPayload,
  type UserRecord,
  type UserStats,
  updateUser,
} from "../api/user.api";

const USER_MANAGEMENT_CACHE_KEY = "manager_user_management_cache_v1";

const defaultStats: UserStats = {
  visibleUsers: 0,
  accountCount: 0,
  privilegedUsers: 0,
  recentLoginUsers: 0,
  activeUsers: 0,
};

const defaultQuery: Required<UserListQuery> = {
  pageIndex: 1,
  pageSize: 10,
  search: "",
  role: "",
  status: "",
};

interface UserManagementCache {
  users: UserRecord[];
  stats: UserStats;
  total: number;
  query: Required<UserListQuery>;
}

export function useUserManagement() {
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [stats, setStats] = useState<UserStats>(defaultStats);
  const [loading, setLoading] = useState(false);
  const [statsLoading, setStatsLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [total, setTotal] = useState(0);
  const [query, setQuery] = useState<Required<UserListQuery>>(defaultQuery);

  const loadUsers = async (nextQuery?: Partial<UserListQuery>) => {
    const mergedQuery = { ...query, ...nextQuery };
    setLoading(true);
    try {
      const result = await fetchUsers(mergedQuery);
      setUsers(result.data);
      setTotal(result.total);
      setQuery(mergedQuery);
      if (typeof window !== "undefined") {
        const rawValue = window.sessionStorage.getItem(USER_MANAGEMENT_CACHE_KEY);
        const parsed = rawValue ? (JSON.parse(rawValue) as UserManagementCache) : null;
        const payload: UserManagementCache = {
          users: result.data,
          stats: parsed?.stats ?? defaultStats,
          total: result.total,
          query: mergedQuery,
        };
        window.sessionStorage.setItem(USER_MANAGEMENT_CACHE_KEY, JSON.stringify(payload));
      }
    } finally {
      setLoading(false);
    }
  };

  const loadStats = async () => {
    setStatsLoading(true);
    try {
      const result = await fetchUserStats();
      setStats(result);
      if (typeof window !== "undefined") {
        const rawValue = window.sessionStorage.getItem(USER_MANAGEMENT_CACHE_KEY);
        const parsed = rawValue ? (JSON.parse(rawValue) as UserManagementCache) : null;
        const payload: UserManagementCache = {
          users: parsed?.users ?? [],
          stats: result,
          total: parsed?.total ?? 0,
          query: parsed?.query ?? defaultQuery,
        };
        window.sessionStorage.setItem(USER_MANAGEMENT_CACHE_KEY, JSON.stringify(payload));
      }
    } finally {
      setStatsLoading(false);
    }
  };

  const refresh = async (nextQuery?: Partial<UserListQuery>) => {
    await Promise.all([loadUsers(nextQuery), loadStats()]);
  };

  const loadDetail = async (id: number) => {
    return fetchUserDetail(id);
  };

  const saveUser = async (id: number | null, payload: UserPayload) => {
    setSubmitting(true);
    try {
      if (id === null) {
        await createUser(payload);
      } else {
        await updateUser(id, payload);
      }
      await refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const patchUser = async (id: number, payload: Partial<UserPayload>) => {
    setSubmitting(true);
    try {
      await updateUser(id, payload);
      await refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const removeUser = async (id: number) => {
    setSubmitting(true);
    try {
      await deleteUser(id);
      const nextPage =
        users.length === 1 && query.pageIndex > 1 ? query.pageIndex - 1 : query.pageIndex;
      await refresh({ pageIndex: nextPage });
    } finally {
      setSubmitting(false);
    }
  };

  useEffect(() => {
    if (typeof window === "undefined") {
      void refresh();
      return;
    }

    try {
      const rawValue = window.sessionStorage.getItem(USER_MANAGEMENT_CACHE_KEY);
      if (rawValue) {
        const parsed = JSON.parse(rawValue) as UserManagementCache;
        setUsers(parsed.users ?? []);
        setStats(parsed.stats ?? defaultStats);
        setTotal(parsed.total ?? 0);
        setQuery(parsed.query ?? defaultQuery);
        return;
      }
    } catch {
      window.sessionStorage.removeItem(USER_MANAGEMENT_CACHE_KEY);
    }

    void refresh();
  }, []);

  return {
    users,
    stats,
    total,
    query,
    loading,
    statsLoading,
    submitting,
    refresh,
    loadUsers,
    loadDetail,
    saveUser,
    patchUser,
    removeUser,
    setQuery,
  };
}
