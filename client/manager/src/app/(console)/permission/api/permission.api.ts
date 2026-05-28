"use client";

import { getPage, instance, unwrapApiResponse, type ApiResponse, type PageResult } from "@/utils/axios";
import type { CrudListQuery } from "../../components/CrudManagementPanel";

export class PermissionResourceRecord {
  id!: number;
  name = "";
  code = "";
  parentId = 0;
  resourceType = "";
  resourceUrl = "";
  pageUrl = "";
  component = "";
  redirect = "";
  menuName = "";
  meta = "";
  sortId = 0;
  createdTime?: string;
  updatedTime?: string;
  [key: string]: unknown;
}

export class PermissionRoleRecord {
  id!: number;
  name = "";
  code = "";
  createdTime?: string;
  updatedTime?: string;
  [key: string]: unknown;
}

export class RoleResourceRecord {
  id!: number;
  roleId = 0;
  resourceId = 0;
  createdTime?: string;
  updatedTime?: string;
  [key: string]: unknown;
}

export interface PermissionRolePayload {
  name: string;
  code: string;
}

export interface RoleResourcePayload {
  roleId: number;
  resourceId: number;
}

export const permissionResourceApi = {
  list: (query: CrudListQuery): Promise<PageResult<PermissionResourceRecord>> =>
    getPage(PermissionResourceRecord, "/resources", query),
};

export const permissionRoleApi = {
  list: (query: CrudListQuery): Promise<PageResult<PermissionRoleRecord>> =>
    getPage(PermissionRoleRecord, "/roles", query),
  create: async (payload: PermissionRolePayload) => {
    const response = await instance.post<ApiResponse<PermissionRoleRecord>>("/roles", payload);
    return unwrapApiResponse(response.data);
  },
  update: async (id: number, payload: Partial<PermissionRolePayload>) => {
    const response = await instance.put<ApiResponse<PermissionRoleRecord>>(`/roles/${id}`, payload);
    return unwrapApiResponse(response.data);
  },
  remove: async (id: number) => {
    const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/roles/${id}`);
    return unwrapApiResponse(response.data);
  },
};

export async function fetchAllRoles() {
  return permissionRoleApi.list({ pageIndex: 1, pageSize: 1000 });
}

export async function fetchAllResources(
  query: Partial<Pick<CrudListQuery, "name" | "code" | "resourceType">> = {},
) {
  return permissionResourceApi.list({ pageIndex: 1, pageSize: 2000, ...query });
}

export async function fetchRoleResources(roleId: number) {
  return getPage(RoleResourceRecord, "/role-resources", { roleId, pageIndex: 1, pageSize: 3000 });
}

export async function createRoleResource(payload: RoleResourcePayload) {
  const response = await instance.post<ApiResponse<RoleResourceRecord>>("/role-resources", payload);
  return unwrapApiResponse(response.data);
}

export async function deleteRoleResource(id: number) {
  const response = await instance.delete<ApiResponse<{ deleted: boolean }>>(`/role-resources/${id}`);
  return unwrapApiResponse(response.data);
}

export async function syncRoleResources(roleId: number, nextResourceIds: number[]) {
  const current = await fetchRoleResources(roleId);
  const currentByResourceId = new Map(current.data.map((item) => [item.resourceId, item]));
  const nextSet = new Set(nextResourceIds);

  const toCreate = nextResourceIds.filter((resourceId) => !currentByResourceId.has(resourceId));
  const toDelete = current.data.filter((item) => !nextSet.has(item.resourceId));

  await Promise.all([
    ...toCreate.map((resourceId) => createRoleResource({ roleId, resourceId })),
    ...toDelete.map((item) => deleteRoleResource(item.id)),
  ]);

  return {
    created: toCreate.length,
    deleted: toDelete.length,
    total: nextResourceIds.length,
  };
}
