import { useLocation } from 'react-router-dom';
import styles from './Topbar.module.css';

export function Topbar() {
  const location = useLocation();
  const crumb = location.pathname.startsWith('/evaluation')
    ? 'Model comparison'
    : location.pathname.startsWith('/dashboard')
      ? 'Dashboard'
      : location.pathname.startsWith('/accounts')
        ? 'Accounts'
        : 'Cases';

  return (
    <header className={`${styles.root} topbar`}>
      <div className="crumbs">
        <span>Workspace</span>
        <span className="sep">/</span>
        <span className="current">{crumb}</span>
      </div>
      <div className="topbar-spacer" />
    </header>
  );
}
