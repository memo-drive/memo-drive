import { createContext, useContext, useMemo, useState, type Dispatch, type SetStateAction } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import styles from "./MobileShell.module.css";

const navItems = [
  { to: "/m", end: true, icon: "cloud", labelKey: "mobile.nav.home" },
  { to: "/m/files", icon: "folder", labelKey: "mobile.nav.files" },
  { to: "/m/ai", icon: "auto_awesome", labelKey: "mobile.nav.ai" },
  { to: "/m/transfer", icon: "swap_vert", labelKey: "mobile.nav.transfer" },
  { to: "/m/me", icon: "person", labelKey: "mobile.nav.me" },
];

interface MobileShellChrome {
  bottomNavHidden: boolean;
  setBottomNavHidden: Dispatch<SetStateAction<boolean>>;
}

const MobileShellChromeContext = createContext<MobileShellChrome | null>(null);

export function MobileShell() {
  const { t } = useTranslation();
  const [bottomNavHidden, setBottomNavHidden] = useState(false);
  const chrome = useMemo(
    () => ({ bottomNavHidden, setBottomNavHidden }),
    [bottomNavHidden],
  );

  return (
    <MobileShellChromeContext.Provider value={chrome}>
      <div className={styles.shell}>
        <main className={styles.main} aria-label={t("mobile.shell.main")}>
          <Outlet />
        </main>
        <nav
          className={`${styles.bottomNav} ${bottomNavHidden ? styles.bottomNavHidden : ""}`}
          aria-label={t("mobile.shell.nav")}
          aria-hidden={bottomNavHidden || undefined}
        >
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `${styles.navItem} ${isActive ? styles.navItemActive : ""}`
              }
            >
              <span className="material-symbols-outlined" aria-hidden>
                {item.icon}
              </span>
              <span>{t(item.labelKey)}</span>
            </NavLink>
          ))}
        </nav>
      </div>
    </MobileShellChromeContext.Provider>
  );
}

export function useMobileShellChrome() {
  return useContext(MobileShellChromeContext) ?? null;
}
