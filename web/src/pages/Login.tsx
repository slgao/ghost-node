import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useVPNStore } from "@/store/vpnStore";
import { ShieldCheck, Eye, EyeOff } from "lucide-react";

type Mode = "login" | "register";

export default function Login() {
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPw, setShowPw] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const { login, register } = useVPNStore();
  const navigate = useNavigate();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      mode === "login" ? await login(email, password) : await register(email, password);
      navigate("/");
    } catch {
      setError(mode === "login" ? "Wrong email or password." : "Could not create account.");
    } finally {
      setLoading(false);
    }
  };

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "10px 12px",
    background: "var(--raised)",
    border: "1px solid var(--border)",
    borderRadius: 8,
    color: "var(--text-1)",
    fontSize: 14,
    outline: "none",
    transition: "border-color .15s",
  };

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 24,
        background: "var(--bg)",
      }}
    >
      <div style={{ width: "100%", maxWidth: 360 }}>
        {/* Brand */}
        <div style={{ textAlign: "center", marginBottom: 32 }}>
          <div
            style={{
              display: "inline-flex",
              width: 40,
              height: 40,
              borderRadius: 10,
              background: "var(--surface)",
              border: "1px solid var(--border)",
              alignItems: "center",
              justifyContent: "center",
              marginBottom: 16,
            }}
          >
            <ShieldCheck size={20} color="var(--green)" strokeWidth={2} />
          </div>
          <h1 style={{ fontSize: 20, fontWeight: 600, color: "var(--text-1)", letterSpacing: "-0.02em" }}>
            VPN Platform
          </h1>
          <p style={{ color: "var(--text-3)", fontSize: 13, marginTop: 4 }}>
            {mode === "login" ? "Sign in to your account" : "Create a new account"}
          </p>
        </div>

        {/* Form */}
        <form onSubmit={submit} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {/* Email */}
          <div>
            <label style={{ display: "block", fontSize: 12, fontWeight: 500, color: "var(--text-2)", marginBottom: 6 }}>
              Email
            </label>
            <input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              style={inputStyle}
              onFocus={(e) => (e.currentTarget.style.borderColor = "var(--text-2)")}
              onBlur={(e) => (e.currentTarget.style.borderColor = "var(--border)")}
            />
          </div>

          {/* Password */}
          <div>
            <label style={{ display: "block", fontSize: 12, fontWeight: 500, color: "var(--text-2)", marginBottom: 6 }}>
              Password
            </label>
            <div style={{ position: "relative" }}>
              <input
                type={showPw ? "text" : "password"}
                required
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Min. 8 characters"
                style={{ ...inputStyle, paddingRight: 40 }}
                onFocus={(e) => (e.currentTarget.style.borderColor = "var(--text-2)")}
                onBlur={(e) => (e.currentTarget.style.borderColor = "var(--border)")}
              />
              <button
                type="button"
                onClick={() => setShowPw((v) => !v)}
                style={{
                  position: "absolute",
                  right: 12,
                  top: "50%",
                  transform: "translateY(-50%)",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  color: "var(--text-3)",
                  display: "flex",
                  alignItems: "center",
                }}
              >
                {showPw ? <EyeOff size={15} /> : <Eye size={15} />}
              </button>
            </div>
          </div>

          {/* Error */}
          {error && (
            <p style={{ fontSize: 13, color: "var(--red)", padding: "8px 12px", background: "rgba(239,68,68,.08)", borderRadius: 6 }}>
              {error}
            </p>
          )}

          {/* Submit */}
          <button
            type="submit"
            disabled={loading}
            style={{
              marginTop: 4,
              padding: "10px 16px",
              background: "var(--text-1)",
              color: "var(--bg)",
              border: "none",
              borderRadius: 8,
              fontWeight: 600,
              fontSize: 14,
              cursor: loading ? "not-allowed" : "pointer",
              opacity: loading ? 0.6 : 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 8,
              transition: "opacity .15s",
            }}
          >
            {loading ? (
              <>
                <span
                  className="spin"
                  style={{
                    width: 14,
                    height: 14,
                    borderRadius: "50%",
                    border: "2px solid rgba(0,0,0,.2)",
                    borderTopColor: "#000",
                    display: "inline-block",
                  }}
                />
                Please wait…
              </>
            ) : mode === "login" ? (
              "Sign in"
            ) : (
              "Create account"
            )}
          </button>
        </form>

        {/* Toggle */}
        <p style={{ textAlign: "center", marginTop: 24, fontSize: 13, color: "var(--text-3)" }}>
          {mode === "login" ? "No account? " : "Already registered? "}
          <button
            onClick={() => { setMode(mode === "login" ? "register" : "login"); setError(""); }}
            style={{ background: "none", border: "none", cursor: "pointer", color: "var(--text-2)", fontWeight: 500, fontSize: 13 }}
          >
            {mode === "login" ? "Register" : "Sign in"}
          </button>
        </p>
      </div>
    </div>
  );
}
