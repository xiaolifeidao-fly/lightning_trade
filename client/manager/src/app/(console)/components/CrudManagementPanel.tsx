"use client";

import { useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import {
  DeleteOutlined,
  EditOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from "@ant-design/icons";
import {
  Button,
  Cascader,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from "antd";
import type { ColumnsType } from "antd/es/table";
import type { PageResult } from "@/utils/axios";

const { Text } = Typography;

export interface CrudRecord {
  id: number;
  createdTime?: string;
  updatedTime?: string;
}

export interface CrudOption {
  label: string;
  value: string | number | boolean;
}

export interface CrudCascaderOption {
  label: string;
  value: string;
  children?: CrudCascaderOption[];
}

export interface CrudField<R extends CrudRecord> {
  name: Extract<keyof R, string> | string;
  label: string;
  type?: "text" | "textarea" | "number" | "select" | "password" | "cascader";
  required?: boolean;
  placeholder?: string;
  options?: CrudOption[];
  cascaderOptions?: CrudCascaderOption[];
  linkedNames?: [Extract<keyof R, string>, Extract<keyof R, string>, Extract<keyof R, string>];
  min?: number;
  precision?: number;
  hiddenOnCreate?: boolean;
  hiddenOnEdit?: boolean;
  disabledOnEdit?: boolean;
}

export interface CrudTableColumn<R extends CrudRecord> {
  name: Extract<keyof R, string>;
  label: string;
  width?: number;
  copyable?: boolean;
  render?: (value: unknown, record: R) => ReactNode;
}

export interface CrudListQuery {
  pageIndex?: number;
  pageSize?: number;
  [key: string]: string | number | undefined;
}

interface CrudApi<R extends CrudRecord, P extends object> {
  list: (query: CrudListQuery) => Promise<PageResult<R>>;
  create: (payload: P) => Promise<unknown>;
  update: (id: number, payload: Partial<P>) => Promise<unknown>;
  remove: (id: number) => Promise<unknown>;
}

interface CrudActionContext {
  reload: () => Promise<void>;
  submitting: boolean;
  setSubmitting: (submitting: boolean) => void;
}

interface CrudManagementPanelProps<R extends CrudRecord, P extends object> {
  title: string;
  createText: string;
  searchPlaceholder: string;
  searchParam: string;
  fields: CrudField<R>[];
  columns: CrudTableColumn<R>[];
  api: CrudApi<R, P>;
  statusField?: Extract<keyof R, string>;
  statusOptions?: CrudOption[];
  filters?: CrudField<R>[];
  rowActions?: (record: R, context: CrudActionContext) => ReactNode;
  actionWidth?: number;
}

const defaultPageSize = 10;

function compactPayload(values: Record<string, unknown>) {
  return Object.fromEntries(
    Object.entries(values).filter(([, value]) => value !== undefined && value !== ""),
  );
}

function compactQueryPayload(values: Record<string, unknown>): CrudListQuery {
  return Object.fromEntries(
    Object.entries(values).filter(([, value]) => {
      return (typeof value === "string" && value !== "") || typeof value === "number";
    }),
  ) as CrudListQuery;
}

function renderValue(value: unknown) {
  if (value === null || value === undefined || value === "") {
    return "-";
  }
  if (typeof value === "boolean") {
    return value ? "是" : "否";
  }
  return String(value);
}

function buildInitialFormValues<R extends CrudRecord>(record: R, fields: CrudField<R>[]) {
  const values = Object.assign({}, record) as unknown as Record<string, unknown>;
  const recordValues = Object(record) as Record<string, unknown>;
  fields.forEach((field) => {
    if (field.type !== "cascader" || !field.linkedNames) {
      return;
    }
    values[field.name] = field.linkedNames
      .map((name) => recordValues[name])
      .filter((value) => typeof value === "string" && value !== "");
  });
  return values;
}

function applyLinkedFieldValues<R extends CrudRecord>(values: Record<string, unknown>, fields: CrudField<R>[]) {
  fields.forEach((field) => {
    if (field.type !== "cascader" || !field.linkedNames) {
      return;
    }
    const fieldName = String(field.name);
    const fieldValue = values[fieldName];
    const selectedValues = Array.isArray(fieldValue) ? fieldValue : [];
    field.linkedNames.forEach((name, index) => {
      values[name] = selectedValues[index] ?? "";
    });
    delete values[fieldName];
  });
}

function statusTag(value: unknown, label?: string) {
  const text = label ?? renderValue(value);
  const normalized = String(value ?? "").toUpperCase();
  const color =
    normalized.includes("FAILED") ||
    normalized.includes("FROZEN") ||
    normalized.includes("EXPIRED") ||
    normalized.includes("OFFLINE") ||
    normalized.includes("INACTIVE")
      ? "red"
      : normalized.includes("LOCKED")
        ? "orange"
        : normalized.includes("SUCCESS") ||
            normalized.includes("NORMAL") ||
            normalized === "ACTIVE" ||
            normalized.includes("LOGGED_IN")
          ? "green"
          : "blue";

  return <Tag color={color}>{text}</Tag>;
}

export function CrudManagementPanel<R extends CrudRecord, P extends object>({
  title,
  createText,
  searchPlaceholder,
  searchParam,
  fields,
  columns,
  api,
  statusField,
  statusOptions,
  filters = [],
  rowActions,
  actionWidth = 132,
}: CrudManagementPanelProps<R, P>) {
  const [form] = Form.useForm();
  const [filterForm] = Form.useForm();
  const [records, setRecords] = useState<R[]>([]);
  const [total, setTotal] = useState(0);
  const [query, setQuery] = useState<Required<Pick<CrudListQuery, "pageIndex" | "pageSize">>>({
    pageIndex: 1,
    pageSize: defaultPageSize,
  });
  const [searchValue, setSearchValue] = useState("");
  const [statusValue, setStatusValue] = useState<string | number | undefined>();
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingRecord, setEditingRecord] = useState<R | null>(null);

  const loadRecords = async (nextQuery?: CrudListQuery) => {
    const mergedQuery = { ...query, ...nextQuery };
    setLoading(true);
    try {
      const result = await api.list(mergedQuery);
      setRecords(result.data);
      setTotal(result.total);
      setQuery({
        pageIndex: Number(mergedQuery.pageIndex ?? 1),
        pageSize: Number(mergedQuery.pageSize ?? defaultPageSize),
      });
    } catch (error) {
      message.error(error instanceof Error ? error.message : "加载数据失败");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadRecords();
  }, []);

  const stats = useMemo(
    () => [
      { label: `${title}总数`, value: total },
      { label: "当前页数量", value: records.length },
      {
        label: "最近更新",
        value: records[0]?.updatedTime ? String(records[0].updatedTime).slice(0, 10) : "-",
      },
    ],
    [records, title, total],
  );

  const tableColumns: ColumnsType<R> = [
    {
      title: "ID",
      dataIndex: "id",
      width: 80,
      fixed: "left",
    },
    ...columns.map((column) => ({
      title: column.label,
      dataIndex: column.name,
      width: column.width,
      render: (value: unknown, record: R) => {
        if (column.render) {
          return column.render(value, record);
        }
        if (column.name === statusField) {
          const option = statusOptions?.find((item) => item.value === value);
          return statusTag(value, option?.label);
        }
        return column.copyable ? (
          <Text copyable style={{ color: "var(--manager-text)" }}>
            {renderValue(value)}
          </Text>
        ) : (
          renderValue(value)
        );
      },
    })),
    {
      title: "操作",
      key: "actions",
      width: actionWidth,
      fixed: "right",
      render: (_, record) => (
        <Space size={4}>
          {rowActions?.(record, {
            reload: () => loadRecords({ pageIndex: query.pageIndex }),
            submitting,
            setSubmitting,
          })}
          <Tooltip title="编辑">
            <Button
              type="text"
              icon={<EditOutlined />}
              onClick={() => {
                setEditingRecord(record);
                form.setFieldsValue(buildInitialFormValues(record, fields));
                setModalOpen(true);
              }}
            />
          </Tooltip>
          <Popconfirm
            title={`确认删除这条${title}记录吗？`}
            okText="删除"
            cancelText="取消"
            onConfirm={async () => {
              try {
                setSubmitting(true);
                await api.remove(record.id);
                message.success("删除成功");
                const nextPage = records.length === 1 && query.pageIndex > 1 ? query.pageIndex - 1 : query.pageIndex;
                await loadRecords({ pageIndex: nextPage });
              } catch (error) {
                message.error(error instanceof Error ? error.message : "删除失败");
              } finally {
                setSubmitting(false);
              }
            }}
          >
            <Tooltip title="删除">
              <Button danger type="text" icon={<DeleteOutlined />} />
            </Tooltip>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  const filterQuery = () => ({
    pageIndex: 1,
    [searchParam]: searchValue.trim() || undefined,
    ...(statusField ? { [statusField]: statusValue } : {}),
    ...compactQueryPayload(filterForm.getFieldsValue()),
  });

  const renderFilterField = (field: CrudField<R>) => {
    if (field.type === "number") {
      return (
        <InputNumber
          min={field.min}
          precision={field.precision}
          placeholder={field.placeholder ?? field.label}
          style={{ width: 160 }}
        />
      );
    }
    if (field.type === "select") {
      return (
        <Select
          allowClear
          placeholder={field.placeholder ?? field.label}
          options={field.options}
          style={{ width: 160 }}
        />
      );
    }
    return <Input placeholder={field.placeholder ?? field.label} style={{ width: 180 }} />;
  };

  const handleSubmit = async () => {
    const rawValues = compactPayload(form.getFieldsValue());
    applyLinkedFieldValues(rawValues, fields);
    if (editingRecord) {
      fields.forEach((field) => {
        if (field.disabledOnEdit) {
          delete rawValues[field.name];
        }
      });
    }
    const values = rawValues as P;
    setSubmitting(true);
    try {
      if (editingRecord) {
        await api.update(editingRecord.id, values);
        message.success("更新成功");
      } else {
        await api.create(values);
        message.success("创建成功");
      }
      setModalOpen(false);
      setEditingRecord(null);
      form.resetFields();
      await loadRecords({ pageIndex: editingRecord ? query.pageIndex : 1 });
    } catch (error) {
      message.error(error instanceof Error ? error.message : "保存失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="manager-page-stack">
      <section
        className="manager-stats-grid"
        style={{ gridTemplateColumns: "repeat(auto-fit, minmax(220px, 1fr))" }}
      >
        {stats.map((item) => (
          <div key={item.label} className="manager-data-card">
            <div className="manager-section-label">{item.label}</div>
            <div className="manager-display-title" style={{ fontSize: 32, marginTop: 12 }}>
              {item.value}
            </div>
          </div>
        ))}
      </section>

      <section className="manager-data-card">
        <div style={{ display: "flex", gap: 12, flexWrap: "wrap", justifyContent: "space-between" }}>
          <Space wrap size={12}>
            <Input
              className="manager-filter-input"
              prefix={<SearchOutlined style={{ color: "var(--manager-text-faint)" }} />}
              placeholder={searchPlaceholder}
              value={searchValue}
              onChange={(event) => setSearchValue(event.target.value)}
              onPressEnter={() => void loadRecords(filterQuery())}
              style={{ width: 260, maxWidth: "100%" }}
            />
            {statusField && statusOptions ? (
              <Select
                allowClear
                placeholder="状态"
                value={statusValue}
                onChange={setStatusValue}
                options={statusOptions}
                style={{ width: 180 }}
              />
            ) : null}
            {filters.length > 0 ? (
              <Form form={filterForm} component={false}>
                <Space wrap size={12}>
                  {filters.map((field) => (
                    <Form.Item key={field.name} name={field.name as string} noStyle>
                      {renderFilterField(field)}
                    </Form.Item>
                  ))}
                </Space>
              </Form>
            ) : null}
            <Button type="primary" icon={<SearchOutlined />} onClick={() => void loadRecords(filterQuery())}>
              查询
            </Button>
            <Button icon={<ReloadOutlined />} onClick={() => void loadRecords()}>
              刷新
            </Button>
          </Space>

          <Space wrap>
            <Tag style={{ color: "var(--manager-primary)", background: "var(--manager-gold-soft)", border: "1px solid rgba(240,185,11,0.28)" }}>
              共 {total} 条
            </Tag>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => {
                setEditingRecord(null);
                form.resetFields();
                setModalOpen(true);
              }}
              style={{
                color: "#0B0E11",
                border: "none",
                background: "linear-gradient(135deg, #FCD535 0%, #F0B90B 100%)",
                fontWeight: 700,
                boxShadow: "0 10px 22px rgba(240, 185, 11, 0.28)",
              }}
            >
              {createText}
            </Button>
          </Space>
        </div>
      </section>

      <section className="manager-data-card manager-table">
        <Table<R>
          rowKey="id"
          loading={loading}
          dataSource={records}
          columns={tableColumns}
          scroll={{ x: Math.max(1100, columns.reduce((sum, item) => sum + (item.width ?? 160), 220)) }}
          pagination={{
            current: query.pageIndex,
            pageSize: query.pageSize,
            total,
            showSizeChanger: false,
            onChange: (page) => void loadRecords({ ...filterQuery(), pageIndex: page }),
          }}
        />
      </section>

      <Modal
        title={editingRecord ? `编辑${title}` : createText}
        open={modalOpen}
        okText="保存"
        cancelText="取消"
        confirmLoading={submitting}
        onCancel={() => {
          setModalOpen(false);
          setEditingRecord(null);
          form.resetFields();
        }}
        onOk={() => void form.validateFields().then(handleSubmit)}
        destroyOnClose
        width={720}
      >
        <Form form={form} layout="vertical" preserve={false} style={{ marginTop: 16 }}>
          {fields
            .filter((field) => !(field.hiddenOnCreate && !editingRecord))
            .filter((field) => !(field.hiddenOnEdit && editingRecord))
            .map((field) => (
              <Form.Item
                key={field.name}
                name={field.name as string}
                label={field.label}
                rules={field.required ? [{ required: true, message: `请输入${field.label}` }] : undefined}
              >
                {field.type === "textarea" ? (
                  <Input.TextArea rows={4} placeholder={field.placeholder} disabled={field.disabledOnEdit && Boolean(editingRecord)} />
                ) : field.type === "number" ? (
                  <InputNumber
                    min={field.min}
                    precision={field.precision}
                    style={{ width: "100%" }}
                    placeholder={field.placeholder}
                    disabled={field.disabledOnEdit && Boolean(editingRecord)}
                  />
                ) : field.type === "select" ? (
                  <Select
                    allowClear
                    placeholder={field.placeholder}
                    options={field.options}
                    disabled={field.disabledOnEdit && Boolean(editingRecord)}
                  />
                ) : field.type === "cascader" ? (
                  <Cascader
                    allowClear
                    showSearch
                    options={field.cascaderOptions}
                    placeholder={field.placeholder}
                    disabled={field.disabledOnEdit && Boolean(editingRecord)}
                    changeOnSelect={false}
                  />
                ) : field.type === "password" ? (
                  <Input.Password placeholder={field.placeholder} disabled={field.disabledOnEdit && Boolean(editingRecord)} />
                ) : (
                  <Input placeholder={field.placeholder} disabled={field.disabledOnEdit && Boolean(editingRecord)} />
                )}
              </Form.Item>
            ))}
        </Form>
      </Modal>
    </div>
  );
}
