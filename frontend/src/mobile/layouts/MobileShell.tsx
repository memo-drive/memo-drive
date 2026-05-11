import { NavLink, Outlet } from "react-router-dom";
import { useTranslation } from "react-i18next";
import styles from "./MobileShell.module.css";

const navItems = [
  { to: "/m", end: true, icon: "folder", labelKey: "mobile.nav.files" },
  { to: "/m/ai", icon: "auto_awesome", labelKey: "mobile.nav.ai" },
  { to: "/m/transfer", icon: "swap_vert", labelKey: "mobile.nav.transfer" },
  { to: "/m/me", icon: "person", labelKey: "mobile.nav.me" },
];

export function MobileShell() {
  const { t } = useTranslation();

  return (
    <div className={styles.shell}>
      <main className={styles.main} aria-label={t("mobile.shell.main")}>
        <Outlet />
      </main>
      <nav className={styles.bottomNav} aria-label={t("mobile.shell.nav")}>
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
  );
}
