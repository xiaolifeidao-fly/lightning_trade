"use client";

import { useEffect, useMemo, useState, type CSSProperties, type Key, type ReactNode } from "react";
import {
  ApartmentOutlined,
  ApiOutlined,
  AppstoreAddOutlined,
  DeleteOutlined,
  EditOutlined,
  FileTextOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SearchOutlined,
  TeamOutlined,
} from "@ant-design/icons";
import {
  Button,
  Empty,
  Form,
  Input,
  List,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Statistic,
  Tag,
  Tooltip,
  Tree,
  Typography,
  message,
} from "antd";
import type { DataNode } from "antd/es/tree";
import {
  fetchAllResources,
  fetchAllRoles,
  fetchRoleResources,
  permissionRoleApi,
  syncRoleResources,
  type PermissionResourceRecord,
  type PermissionRolePayload,
  type PermissionRoleRecord,
} from "../api/permission.api";

const { Text, Title } = Typography;

type RoleModalMode = "create" | "edit";

interface RoleFormValue {
  name: string;
  code: string;
}

const resourceTypeOptions = [
  { label: "菜单", value: "menu" },
  { label: "接口", value: "api" },
  { label: "按钮", value: "button" },
  { label: "页面", value: "page" },
  { label: "数据", value: "data" },
];

const resourceTypeMeta: Record<string, { label: string; color: string; icon: ReactNode }> = {
  menu: { label: "菜单", color: "green", icon: <ApartmentOutlined /> },
  api: { label: "接口", color: "blue", icon: <ApiOutlined /> },
  button: { label: "按钮", color: "gold", icon: <AppstoreAddOutlined /> },
  page: { label: "页面", color: "purple", icon: <FileTextOutlined /> },
  data: { label: "数据", color: "cyan", icon: <SafetyCertificateOutlined /> },
};

const panelGridStyle = {
  display: "grid",
  gridTemplateColumns: "minmax(250px, 0.8fr) minmax(420px, 1.45fr) minmax(300px, 0.9fr)",
  gap: 16,
  alignItems: "start",
} satisfies CSSProperties;

function normalizeType(type?: string) {
  const value = String(type ?? "").trim().toLowerCase();
  return value || "api";
}

function getTypeMeta(type?: string) {
  return resourceTypeMeta[normalizeType(type)] ?? {
    label: type || "资源",
    color: "default",
    icon: <SafetyCertificateOutlined />,
  };
}

function renderResourceTitle(resource: PermissionResourceRecord) {
  const meta = getTypeMeta(resource.resourceType);
  const subtitle = resource.resourceUrl || resource.pageUrl || resource.code || "未设置访问路径";

  return (
    <div style={{ display: "flex", gap: 10, alignItems: "center", minWidth: 0 }}>
      <span style={{ color: "var(--manager-primary)", display: "inline-flex" }}>{meta.icon}</span>
      <div style={{ minWidth: 0 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <Text strong style={{ color: "var(--manager-text)" }}>
            {resource.name || resource.menuName || resource.code || `资源 #${resource.id}`}
          </Text>
          <Tag color={meta.color} style={{ margin: 0 }}>
            {meta.label}
          </Tag>
        </div>
        <Text type="secondary" style={{ fontSize: 12 }}>
          {subtitle}
        </Text>
      </div>
    </div>
  );
}

function buildResourceTree(resources: PermissionResourceRecord[]): DataNode[] {
  const ordered = [...resources].sort((a, b) => {
    const sortGap = (a.sortId ?? 0) - (b.sortId ?? 0);
    return sortGap || a.id - b.id;
  });
  const childrenByParent = new Map<number, PermissionResourceRecord[]>();
  const resourceIds = new Set(ordered.map((item) => item.id));

  ordered.forEach((resource) => {
    const parentId = resource.parentId && resourceIds.has(resource.parentId) ? resource.parentId : 0;
    const siblings = childrenByParent.get(parentId) ?? [];
    siblings.push(resource);
    childrenByParent.set(parentId, siblings);
  });

  const toNode = (resource: PermissionResourceRecord): DataNode => ({
    key: resource.id,
    title: renderResourceTitle(resource),
    children: (childrenByParent.get(resource.id) ?? []).map(toNode),
  });

  return (childrenByParent.get(0) ?? []).map(toNode);
}

function roleToFormValue(role?: PermissionRoleRecord | null): Partial<RoleFormValue> {
  if (!role) {
    return {};
  }
  return {
    name: role.name,
    code: role.code,
  };
}

function toNumberKeys(keys: Key[]) {
  return keys.map((key) => Number(key)).filter((key) => Number.isFinite(key) && key > 0);
}

export function PermissionManagementPanel() {
  const [roleForm] = Form.useForm<RoleFormValue>();
  const [roles, setRoles] = useState<PermissionRoleRecord[]>([]);
  const [resources, setResources] = useState<PermissionResourceRecord[]>([]);
  const [checkedResourceIds, setCheckedResourceIds] = useState<number[]>([]);
  const [savedResourceIds, setSavedResourceIds] = useState<number[]>([]);
  const [selectedRoleId, setSelectedRoleId] = useState<number | null>(null);
  const [selectedResourceId, setSelectedResourceId] = useState<number | null>(null);
  const [roleSearch, setRoleSearch] = useState("");
  const [resourceSearch, setResourceSearch] = useState("");
  const [resourceType, setResourceType] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [bindingLoading, setBindingLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [roleModalOpen, setRoleModalOpen] = useState(false);
  const [roleModalMode, setRoleModalMode] = useState<RoleModalMode>("create");
  const [editingRole, setEditingRole] = useState<PermissionRoleRecord | null>(null);

  const selectedRole = useMemo(
    () => roles.find((role) => role.id === selectedRoleId) ?? null,
    [roles, selectedRoleId],
  );
  const selectedResource = useMemo(
    () => resources.find((resource) => resource.id === selectedResourceId) ?? null,
    [resources, selectedResourceId],
  );
  const checkedSet = useMemo(() => new Set(checkedResourceIds), [checkedResourceIds]);
  const unsaved = useMemo(() => {
    const saved = new Set(savedResourceIds);
    return checkedResourceIds.length !== savedResourceIds.length || checkedResourceIds.some((id) => !saved.has(id));
  }, [checkedResourceIds, savedResourceIds]);

  const filteredRoles = useMemo(() => {
    const keyword = roleSearch.trim().toLowerCase();
    if (!keyword) {
      return roles;
    }
    return roles.filter((role) => `${role.name} ${role.code}`.toLowerCase().includes(keyword));
  }, [roleSearch, roles]);

  const filteredResources = useMemo(() => {
    const keyword = resourceSearch.trim().toLowerCase();
    return resources.filter((resource) => {
      const hitKeyword =
        !keyword ||
        `${resource.name} ${resource.code} ${resource.resourceUrl} ${resource.pageUrl} ${resource.menuName}`
          .toLowerCase()
          .includes(keyword);
      const hitType = !resourceType || normalizeType(resource.resourceType) === resourceType;
      return hitKeyword && hitType;
    });
  }, [resourceSearch, resourceType, resources]);

  const treeData = useMemo(() => buildResourceTree(filteredResources), [filteredResources]);
  const selectedRoleResourceCount = useMemo(
    () => resources.filter((resource) => checkedSet.has(resource.id)).length,
    [checkedSet, resources],
  );

  const loadResources = async () => {
    const result = await fetchAllResources();
    setResources(result.data);
    if (result.data.length > 0) {
      setSelectedResourceId((current) => current ?? result.data[0].id);
    }
  };

  const loadRoles = async () => {
    const result = await fetchAllRoles();
    setRoles(result.data);
    setSelectedRoleId((current) => current ?? result.data[0]?.id ?? null);
  };

  const loadAll = async () => {
    setLoading(true);
    try {
      await Promise.all([loadRoles(), loadResources()]);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载权限数据失败");
    } finally {
      setLoading(false);
    }
  };

  const loadRoleBindings = async (roleId: number) => {
    setBindingLoading(true);
    try {
      const result = await fetchRoleResources(roleId);
      const nextIds = result.data.map((item) => item.resourceId);
      setCheckedResourceIds(nextIds);
      setSavedResourceIds(nextIds);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载角色资源失败");
      setCheckedResourceIds([]);
      setSavedResourceIds([]);
    } finally {
      setBindingLoading(false);
    }
  };

  useEffect(() => {
    void loadAll();
  }, []);

  useEffect(() => {
    if (!selectedRoleId) {
      setCheckedResourceIds([]);
      setSavedResourceIds([]);
      return;
    }
    void loadRoleBindings(selectedRoleId);
  }, [selectedRoleId]);

  const openRoleModal = (mode: RoleModalMode, role?: PermissionRoleRecord) => {
    setRoleModalMode(mode);
    setEditingRole(role ?? null);
    roleForm.setFieldsValue(roleToFormValue(role));
    setRoleModalOpen(true);
  };

  const submitRole = async () => {
    const values = await roleForm.validateFields();
    setSubmitting(true);
    try {
      const payload: PermissionRolePayload = {
        name: values.name.trim(),
        code: values.code.trim(),
      };
      if (roleModalMode === "edit" && editingRole) {
        await permissionRoleApi.update(editingRole.id, payload);
        message.success("角色已更新");
      } else {
        const created = await permissionRoleApi.create(payload);
        setSelectedRoleId(created.id);
        message.success("角色已创建");
      }
      setRoleModalOpen(false);
      roleForm.resetFields();
      await loadRoles();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存角色失败");
    } finally {
      setSubmitting(false);
    }
  };

  const removeRole = async (role: PermissionRoleRecord) => {
    setSubmitting(true);
    try {
      await permissionRoleApi.remove(role.id);
      message.success("角色已删除");
      if (selectedRoleId === role.id) {
        setSelectedRoleId(null);
      }
      await loadRoles();
    } catch (error) {
      message.error(error instanceof Error ? error.message : "删除角色失败");
    } finally {
      setSubmitting(false);
    }
  };

  const saveBindings = async () => {
    if (!selectedRoleId) {
      message.warning("请先选择角色");
      return;
    }
    setSubmitting(true);
    try {
      const result = await syncRoleResources(selectedRoleId, checkedResourceIds);
      setSavedResourceIds(checkedResourceIds);
      message.success(`授权已保存，新增 ${result.created} 项，移除 ${result.deleted} 项`);
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存授权失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="manager-page-stack">
      <section
        className="manager-stats-grid"
        style={{ gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))" }}
      >
        <div className="manager-data-card">
          <Statistic title="角色数量" value={roles.length} prefix={<TeamOutlined />} />
        </div>
        <div className="manager-data-card">
          <Statistic title="资源点数量" value={resources.length} prefix={<SafetyCertificateOutlined />} />
        </div>
        <div className="manager-data-card">
          <Statistic title="当前角色授权" value={selectedRoleResourceCount} suffix="项" prefix={<ApiOutlined />} />
        </div>
        <div className="manager-data-card">
          <Statistic title="未保存变更" value={unsaved ? 1 : 0} suffix={unsaved ? "处" : "处"} />
        </div>
      </section>

      <section className="permission-workbench" style={panelGridStyle}>
        <div className="manager-data-card" style={{ minHeight: 620 }}>
          <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center" }}>
            <div>
              <div className="manager-section-label">角色</div>
              <Title level={4} style={{ margin: "6px 0 0" }}>
                选择授权对象
              </Title>
            </div>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => openRoleModal("create")}>
              新增
            </Button>
          </div>
          <Input
            className="manager-filter-input"
            prefix={<SearchOutlined style={{ color: "var(--manager-text-faint)" }} />}
            placeholder="搜索角色名称/编码"
            value={roleSearch}
            onChange={(event) => setRoleSearch(event.target.value)}
            style={{ marginTop: 16 }}
          />
          <Spin spinning={loading}>
            <List
              style={{ marginTop: 14 }}
              dataSource={filteredRoles}
              locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无角色" /> }}
              renderItem={(role) => {
                const active = selectedRoleId === role.id;
                return (
                  <List.Item
                    className={active ? "permission-role-item permission-role-item-active" : "permission-role-item"}
                    onClick={() => setSelectedRoleId(role.id)}
                    actions={[
                      <Tooltip title="编辑" key="edit">
                        <Button
                          type="text"
                          icon={<EditOutlined />}
                          onClick={(event) => {
                            event.stopPropagation();
                            openRoleModal("edit", role);
                          }}
                        />
                      </Tooltip>,
                      <Popconfirm
                        key="delete"
                        title="确认删除这个角色吗？"
                        okText="删除"
                        cancelText="取消"
                        onConfirm={(event) => {
                          event?.stopPropagation();
                          void removeRole(role);
                        }}
                      >
                        <Tooltip title="删除">
                          <Button
                            danger
                            type="text"
                            icon={<DeleteOutlined />}
                            onClick={(event) => event.stopPropagation()}
                          />
                        </Tooltip>
                      </Popconfirm>,
                    ]}
                  >
                    <List.Item.Meta
                      avatar={<TeamOutlined style={{ color: active ? "#0B0E11" : "var(--manager-primary)" }} />}
                      title={<Text strong>{role.name || role.code}</Text>}
                      description={<Text type="secondary">{role.code || `角色 #${role.id}`}</Text>}
                    />
                  </List.Item>
                );
              }}
            />
          </Spin>
        </div>

        <div className="manager-data-card" style={{ minHeight: 620 }}>
          <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "start" }}>
            <div>
              <div className="manager-section-label">资源授权</div>
              <Title level={4} style={{ margin: "6px 0 0" }}>
                {selectedRole ? `${selectedRole.name || selectedRole.code} 的资源点` : "请选择角色"}
              </Title>
              <Text type="secondary">勾选资源树后保存，支持按类型和关键字快速定位。</Text>
            </div>
            <Space wrap>
              <Button icon={<ReloadOutlined />} onClick={() => void loadAll()}>
                刷新
              </Button>
              <Button
                type="primary"
                icon={<SaveOutlined />}
                disabled={!selectedRoleId || !unsaved}
                loading={submitting}
                onClick={() => void saveBindings()}
              >
                保存授权
              </Button>
            </Space>
          </div>

          <div style={{ display: "flex", gap: 12, flexWrap: "wrap", marginTop: 18 }}>
            <Input
              className="manager-filter-input"
              prefix={<SearchOutlined style={{ color: "var(--manager-text-faint)" }} />}
              placeholder="搜索资源名称/编码/URL"
              value={resourceSearch}
              onChange={(event) => setResourceSearch(event.target.value)}
              style={{ flex: "1 1 220px" }}
            />
            <Select
              allowClear
              placeholder="资源类型"
              value={resourceType}
              options={resourceTypeOptions}
              onChange={setResourceType}
              style={{ width: 150 }}
            />
          </div>

          <Spin spinning={loading || bindingLoading}>
            <div className="permission-tree-panel">
              {treeData.length > 0 ? (
                <Tree
                  checkable
                  blockNode
                  showLine
                  defaultExpandAll
                  checkedKeys={checkedResourceIds}
                  selectedKeys={selectedResourceId ? [selectedResourceId] : []}
                  treeData={treeData}
                  onCheck={(keys) => {
                    const nextKeys = Array.isArray(keys) ? keys : keys.checked;
                    setCheckedResourceIds(toNumberKeys(nextKeys));
                  }}
                  onSelect={(keys) => setSelectedResourceId(keys.length > 0 ? Number(keys[0]) : null)}
                />
              ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的资源点" />
              )}
            </div>
          </Spin>
        </div>

        <div className="manager-data-card" style={{ minHeight: 620 }}>
          <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center" }}>
            <div>
              <div className="manager-section-label">资源点</div>
              <Title level={4} style={{ margin: "6px 0 0" }}>
                资源点详情
              </Title>
            </div>
          </div>

          {selectedResource ? (
            <div style={{ marginTop: 18 }}>
              <Space wrap style={{ marginBottom: 12 }}>
                <Tag color={getTypeMeta(selectedResource.resourceType).color}>
                  {getTypeMeta(selectedResource.resourceType).label}
                </Tag>
                <Tag>排序 {selectedResource.sortId ?? 0}</Tag>
                {checkedSet.has(selectedResource.id) ? <Tag color="green">当前角色已授权</Tag> : <Tag>未授权</Tag>}
              </Space>
              <Title level={5} style={{ margin: "0 0 8px" }}>
                {selectedResource.name || selectedResource.menuName || selectedResource.code}
              </Title>
              <div className="permission-detail-list">
                <DetailRow label="资源编码" value={selectedResource.code} />
                <DetailRow label="接口 URL" value={selectedResource.resourceUrl} copyable />
                <DetailRow label="页面 URL" value={selectedResource.pageUrl} copyable />
                <DetailRow label="父级 ID" value={selectedResource.parentId || "无"} />
                <DetailRow label="组件" value={selectedResource.component} />
                <DetailRow label="菜单名" value={selectedResource.menuName} />
                <DetailRow label="元信息" value={selectedResource.meta} />
              </div>
              <Text type="secondary" style={{ display: "block", marginTop: 18 }}>
                资源点由 SQL 初始化维护，此处仅用于查看和给角色授权。
              </Text>
            </div>
          ) : (
            <Empty style={{ marginTop: 64 }} image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择资源点" />
          )}
        </div>
      </section>

      <Modal
        title={roleModalMode === "edit" ? "编辑角色" : "新增角色"}
        open={roleModalOpen}
        okText="保存"
        cancelText="取消"
        confirmLoading={submitting}
        destroyOnClose
        onOk={() => void submitRole()}
        onCancel={() => {
          setRoleModalOpen(false);
          roleForm.resetFields();
        }}
      >
        <Form form={roleForm} layout="vertical" preserve={false} style={{ marginTop: 16 }}>
          <Form.Item name="name" label="角色名称" rules={[{ required: true, message: "请输入角色名称" }]}>
            <Input placeholder="例如：系统管理员" />
          </Form.Item>
          <Form.Item name="code" label="角色编码" rules={[{ required: true, message: "请输入角色编码" }]}>
            <Input placeholder="例如：admin" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}

function DetailRow({ label, value, copyable }: { label: string; value?: ReactNode; copyable?: boolean }) {
  const content = value === undefined || value === null || value === "" ? "-" : value;
  return (
    <div className="permission-detail-row">
      <Text type="secondary">{label}</Text>
      {copyable && typeof content === "string" && content !== "-" ? (
        <Text copyable style={{ color: "var(--manager-text)" }}>
          {content}
        </Text>
      ) : (
        <Text style={{ color: "var(--manager-text)" }}>{content}</Text>
      )}
    </div>
  );
}
