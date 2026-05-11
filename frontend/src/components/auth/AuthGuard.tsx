import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useLocation } from "react-router-dom";
import { getToken, httpClient } from "../../api/client";
import { loginHrefForRedirect } from "./authRedirect";

interface Props {
  children: ReactNode;
}

export function AuthGuard({ children }: Props) {
  const { t } = useTranslation();
  const location = useLocation();
  const [status, setStatus] = useState<"checking" | "authed" | "unauthed">("checking");

  useEffect(() => {
    httpClient
      .checkAuth()
      .then((res) => {
        if (res.required && !getToken()) {
          setStatus("unauthed");
        } else {
          setStatus("authed");
        }
      })
      .catch(() => setStatus("unauthed"));
  }, []);

  if (status === "checking") {
    return (
      <div className="flex items-center justify-center h-screen text-sm text-gray-500">
        {t("auth.wakingUp")}
      </div>
    );
  }

  if (status === "unauthed") {
    return (
      <Navigate
        to={loginHrefForRedirect(location.pathname, location.search, location.hash)}
        replace
      />
    );
  }

  return <>{children}</>;
}
