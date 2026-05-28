"use client";

import { LoginFormCard } from "./components/LoginFormCard";

export default function LoginPage() {
  return (
    <main className="manager-login-shell">
      <section
        className="manager-grid-bg manager-login-panel"
        style={{
          width: "100%",
        }}
      >
        <div
          style={{
            position: "relative",
            zIndex: 1,
            minHeight: "calc(100vh - 96px)",
            display: "grid",
            placeItems: "center",
          }}
        >
          <div style={{ width: "100%", maxWidth: 440 }}>
            <LoginFormCard />
          </div>
        </div>
      </section>
    </main>
  );
}
