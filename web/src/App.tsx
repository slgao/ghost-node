import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { useVPNStore } from "@/store/vpnStore";
import Login from "@/pages/Login";
import Dashboard from "@/pages/Dashboard";

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const isAuthenticated = useVPNStore((s) => s.isAuthenticated);
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" replace />;
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          path="/"
          element={
            <ProtectedRoute>
              <Dashboard />
            </ProtectedRoute>
          }
        />
      </Routes>
    </BrowserRouter>
  );
}
