import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type PropsWithChildren,
} from "react";
import { api, jsonBody } from "../lib/api";
import type { BuildInfo, PublicSystem, User } from "../types";

type AuthContextValue = {
  user: User | null;
  build: BuildInfo | null;
  system: PublicSystem | null;
  loading: boolean;
  login: (identity: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: PropsWithChildren) {
  const [user, setUser] = useState<User | null>(null);
  const [build, setBuild] = useState<BuildInfo | null>(null);
  const [system, setSystem] = useState<PublicSystem | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    const publicInfo = await api<PublicSystem>("/api/v1/system/public");
    setSystem(publicInfo);
    try {
      const me = await api<{ user: User; build: BuildInfo }>("/api/v1/auth/me");
      setUser(me.user);
      setBuild(me.build);
    } catch {
      setUser(null);
      setBuild(null);
    }
  }, []);

  useEffect(() => {
    refresh().finally(() => setLoading(false));
  }, [refresh]);

  const login = useCallback(
    async (identity: string, password: string) => {
      const nextUser = await api<User>("/api/v1/auth/login", {
        method: "POST",
        ...jsonBody({ identity, password }),
      });
      setUser(nextUser);
      await refresh();
    },
    [refresh],
  );

  const logout = useCallback(async () => {
    await api<void>("/api/v1/auth/logout", { method: "POST" });
    setUser(null);
    setBuild(null);
  }, []);

  const value = useMemo(
    () => ({ user, build, system, loading, login, logout, refresh }),
    [user, build, system, loading, login, logout, refresh],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used within AuthProvider");
  return value;
}
