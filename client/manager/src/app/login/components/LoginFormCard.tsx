"use client";

import {
  LockOutlined,
  MailOutlined,
} from "@ant-design/icons";
import { Button, Checkbox, Form, Input, Space, Typography, message } from "antd";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { login } from "@/app/login/api/login.api";
import { isAuthenticated, setAuthToken } from "@/utils/auth";

const { Title } = Typography;

interface LoginValues {
  account: string;
  password: string;
  remember: boolean;
}

export function LoginFormCard() {
  const router = useRouter();
  const [messageApi, contextHolder] = message.useMessage();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (isAuthenticated()) {
      router.replace("/manager-dashboard");
    }
  }, [router]);

  const handleFinish = async (values: LoginValues) => {
    setSubmitting(true);
    try {
      const response = await login({
        username: values.account.trim(),
        password: values.password,
      });
      setAuthToken(response.token, values.remember);
      messageApi.success("登录成功，正在进入后台");
      router.replace("/manager-dashboard");
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : "请输入登录密码";
      messageApi.error(errorMessage);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      {contextHolder}
      <div
        className="manager-shell-card manager-stagger-4 manager-form-skin manager-brand-frame"
        style={{
          borderRadius: 8,
          padding: "32px 32px 30px",
          background: "#ffffff",
        }}
      >
        <Space direction="vertical" size={8} style={{ width: "100%", marginBottom: 24 }}>
          <div className="manager-brand-chip" style={{ margin: "0 auto" }}>
            <span className="manager-brand-dot" />
            收粮管理端
          </div>
        </Space>
        <Title
          level={3}
          className="manager-display-title"
          style={{
            marginTop: 0,
            marginBottom: 6,
            color: "var(--manager-text)",
            textAlign: "center",
          }}
        >
          后台登录
        </Title>
        <Typography.Text
          style={{
            display: "block",
            marginBottom: 26,
            textAlign: "center",
            color: "var(--manager-text-soft)",
          }}
        >
          进入粮站业务数据工作台
        </Typography.Text>

        <Form<LoginValues>
          layout="vertical"
          initialValues={{
            remember: true,
          }}
          onFinish={handleFinish}
        >
          <Form.Item
            label="账号"
            name="account"
            rules={[{ required: true, message: "请输入登录账号" }]}
          >
            <Input
              prefix={<MailOutlined style={{ color: "rgba(16,40,64,0.42)" }} />}
              placeholder="请输入邮箱或账号"
              size="large"
            />
          </Form.Item>

          <Form.Item
            label="密码"
            name="password"
            rules={[{ required: true, message: "请输入登录密码" }]}
          >
            <Input.Password
              prefix={<LockOutlined style={{ color: "rgba(16,40,64,0.42)" }} />}
              placeholder="请输入密码"
              size="large"
            />
          </Form.Item>

          <div style={{ marginBottom: 24 }}>
            <Form.Item name="remember" valuePropName="checked" noStyle>
              <Checkbox>记住登录状态</Checkbox>
            </Form.Item>
          </div>

          <Button
            type="primary"
            htmlType="submit"
            block
            size="large"
            loading={submitting}
            style={{
              height: 50,
              color: "#ffffff",
              background: "linear-gradient(135deg, #145535 0%, #237a4b 100%)",
              border: "none",
              fontWeight: 800,
            }}
          >
            登录后台
          </Button>
        </Form>
      </div>
    </>
  );
}
