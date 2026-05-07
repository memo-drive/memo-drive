import { FormEvent, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useNavigate } from "react-router-dom";
import { getToken, httpClient } from "../../api/client";
import { Button } from "../../components/base";
import styles from "./index.module.css";

export function LoginPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");

  if (getToken()) {
    return <Navigate to="/" replace />;
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    try {
      await httpClient.login(password);
      navigate("/", { replace: true });
    } catch (err: any) {
      setError(err.message || "Login failed");
    }
  };

  return (
    <div className={styles.pageContainer}>
      <div className={styles.loginCard}>
        <div className="mb-8 flex flex-col items-center">
          <div className={styles.logoBox}>
            <img src="/assets/img/logo.png" alt="MemoDrive" />
          </div>
          <h1 className={styles.title}>MemoDrive</h1>
          <p className={styles.subtitle}>
            {t("auth.subtitle")}
          </p>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          <div>
            <div className="flex justify-between items-center mb-1">
              <label className={styles.label} htmlFor="password">
                {t("auth.password")}
              </label>
            </div>
            <input
              id="password"
              type="password"
              className={styles.input}
              value={password}
              onChange={(e: any) => setPassword(e.target.value)}
              autoFocus
            />
          </div>

          {error && (
            <p className="text-red-600 font-bold text-sm text-center">
              {error}
            </p>
          )}

          <Button type="submit" block className={styles.submitBtn}>
            {t("auth.login")}
            <span className="material-symbols-outlined text-[18px]">
              arrow_forward
            </span>
          </Button>
        </form>

        <footer className="mt-8 text-center flex gap-4 text-sm text-zinc-400">
          <span>{t("auth.private")}</span>
          <span>{t("auth.secure")}</span>
          <span>{t("auth.encrypted")}</span>
        </footer>
      </div>

      {/* Background elements */}
      <div className={styles.backgroundBlob1}></div>
      <div className={styles.backgroundBlob2}></div>
    </div>
  );
}
