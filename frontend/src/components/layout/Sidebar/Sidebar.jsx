import { Link, useLocation } from 'react-router-dom';
import { Icon } from '../../ui';
import styles from './Sidebar.module.css';

const NAV_ITEMS = [
  { to: '/cases', label: 'Cases queue', icon: 'queue' },
  { to: '/dashboard', label: 'Dashboard', icon: 'dashboard' },
  { to: '/accounts', label: 'Accounts', icon: 'users' },
  { to: '/evaluation', label: 'Model comparison', icon: 'chart' },
];

export function Sidebar() {
  const location = useLocation();

  return (
    <aside className={`${styles.root} sidebar`}>
      <div className="brand">
        <div className="brand-mark" />
        <div>
          <div className="brand-name">case-detect</div>
          <div className="brand-sub">case-level analyst panel</div>
        </div>
      </div>

      <div className="nav-section">
        <div className="nav-label">Workspace</div>
        {NAV_ITEMS.map((item) => (
          <Link
            key={item.to}
            className={`nav-item ${location.pathname.startsWith(item.to) ? 'active' : ''}`}
            to={item.to}
          >
            <Icon name={item.icon} />
            <span>{item.label}</span>
          </Link>
        ))}
      </div>

      <div className="nav-section">
        <div className="nav-label">Next stage</div>
        <div className="nav-item disabled">
          <Icon name="live" />
          <span>Live ingest</span>
          <span className="nav-count">soon</span>
        </div>
      </div>

      <div className="sidebar-footer">
        <span className="dot-live" />
        <span>API · localhost:8080</span>
      </div>
    </aside>
  );
}
