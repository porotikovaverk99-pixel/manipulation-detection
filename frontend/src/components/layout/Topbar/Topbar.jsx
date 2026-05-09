import { useLocation } from 'react-router-dom';
import { SCORERS } from '../../../services/api';
import styles from './Topbar.module.css';

export function Topbar({ scorerKey, setScorerKey }) {
  const location = useLocation();
  const crumb = location.pathname.startsWith('/evaluation')
    ? 'Model comparison'
    : location.pathname.startsWith('/dashboard')
      ? 'Dashboard'
      : 'Cases';

  return (
    <header className={`${styles.root} topbar`}>
      <div className="crumbs">
        <span>Workspace</span>
        <span className="sep">/</span>
        <span className="current">{crumb}</span>
      </div>
      <div className="topbar-spacer" />
      <label className="select-chip">
        <span>scorer</span>
        <select value={scorerKey} onChange={(event) => setScorerKey(event.target.value)}>
          {SCORERS.map((scorer) => (
            <option key={scorer.key} value={scorer.key}>
              {scorer.label}
            </option>
          ))}
        </select>
      </label>
    </header>
  );
}
