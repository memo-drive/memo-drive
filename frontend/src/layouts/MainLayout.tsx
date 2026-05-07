import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { clearToken } from "../api/client";
import { Button } from "../components/base";
import styles from "./MainLayout.module.css";

const navLinkBase =
  "flex items-center gap-3 px-3 py-2 rounded-md transition-all duration-150 ease-in-out text-[14px]";

export function MainLayout() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleLogout = () => {
    clearToken();
    navigate("/login", { replace: true });
  };

  return (
    <div className={styles.appShell}>
      {/* Sidebar Navigation */}
      <aside className={styles.sidebar}>
        <NavLink to="/" end className="px-4 py-6 flex items-center gap-3">
          <div className="w-8 h-8 rounded bg-primary-container flex items-center justify-center text-white">
            <img src="/assets/img/logo.png" alt="" />
          </div>
          <div>
            <h1 className="text-md font-bold tracking-tight text-zinc-800 font-display-secondary">
              MemoDrive
            </h1>
            <p className="text-[12px] text-zinc-500 font-medium">
              {t("layout.tagline")}
            </p>
          </div>
        </NavLink>

        <nav className="flex-1 px-2 space-y-1">
          <NavLink
            to="/"
            end
            className={({ isActive }: { isActive: boolean }) =>
              `${navLinkBase} ${
                isActive
                  ? "bg-zinc-200/50 text-zinc-900 font-medium"
                  : "text-zinc-600 hover:bg-zinc-200/30"
              }`
            }
          >
            <span className="material-symbols-outlined text-[20px]">
              folder
            </span>
            {t("layout.allFiles")}
          </NavLink>
          <NavLink
            to="/smart-search"
            className={({ isActive }: { isActive: boolean }) =>
              `${navLinkBase} ${
                isActive
                  ? "bg-zinc-200/50 text-zinc-900 font-medium"
                  : "text-zinc-600 hover:bg-zinc-200/30"
              }`
            }
          >
            <span className="material-symbols-outlined text-[20px]">
              auto_awesome
            </span>
            {t("layout.smartSearch")}
          </NavLink>
          <NavLink
            to="/transfer"
            className={({ isActive }: { isActive: boolean }) =>
              `${navLinkBase} ${
                isActive
                  ? "bg-zinc-200/50 text-zinc-900 font-medium"
                  : "text-zinc-600 hover:bg-zinc-200/30"
              }`
            }
          >
            <span className="material-symbols-outlined text-[20px]">
              swap_vert
            </span>
            {t("layout.transfer")}
          </NavLink>
          <NavLink
            to="/trash"
            className={({ isActive }: { isActive: boolean }) =>
              `${navLinkBase} ${
                isActive
                  ? "bg-zinc-200/50 text-zinc-900 font-medium"
                  : "text-zinc-600 hover:bg-zinc-200/30"
              }`
            }
          >
            <span className="material-symbols-outlined text-[20px]">
              delete
            </span>
            {t("layout.trash")}
          </NavLink>
        </nav>

        <div className="px-2 pb-4 pt-4 border-t border-zinc-100 space-y-1">
          <Button
            onClick={handleLogout}
            variant="ghost"
            block
            className="!text-red-600 hover:!bg-red-50 !justify-start"
          >
            <span className="material-symbols-outlined text-[20px]">
              logout
            </span>
            <span className="text-[14px]">{t("layout.logout")}</span>
          </Button>
        </div>
      </aside>

      {/* Main Content Wrapper */}
      <main className={styles.mainContent}>
        {/* Top App Bar */}
        <header className={styles.topbar}>
          <div className="flex items-center flex-1 max-w-xl"></div>
          <div className="flex items-center gap-4 ml-6">
            <div className="flex items-center gap-1 border-l border-zinc-200 pl-4">
              <Button
                variant="ghost"
                className="!p-2"
                onClick={() => navigate("/settings")}
              >
                <span className="material-symbols-outlined">
                  settings
                </span>
              </Button>
            </div>
          </div>
        </header>

        {/* Content Area */}
        <div className={styles.contentArea}>
          <Outlet />
        </div>
      </main>
    </div>
  );
}
