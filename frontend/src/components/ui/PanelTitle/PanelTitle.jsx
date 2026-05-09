import styles from './PanelTitle.module.css';

export function PanelTitle({ title, subtitle }) {
  return (
    <div className={`${styles.root} panel-title`}>
      <h2>{title}</h2>
      {subtitle && <span>{subtitle}</span>}
    </div>
  );
}
