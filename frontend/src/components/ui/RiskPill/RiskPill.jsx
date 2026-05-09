import styles from './RiskPill.module.css';
import { riskClass } from '../../../utils/format';

export function RiskPill({ level }) {
  return <span className={`${styles.root} risk-pill ${riskClass(level)}`}>{level}</span>;
}
