import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import './App.css';
import { Sidebar, Topbar } from './components/layout';
import { Accounts, CaseDetails, CasesQueue, Dashboard, ModelComparison } from './pages';

function App() {
  return (
    <BrowserRouter>
      <div className="app-shell">
        <Sidebar />
        <main className="main">
          <Topbar />
          <Routes>
            <Route path="/" element={<Navigate to="/cases" replace />} />
            <Route path="/cases" element={<CasesQueue />} />
            <Route path="/cases/:id" element={<CaseDetails />} />
            <Route path="/dashboard" element={<Dashboard />} />
            <Route path="/accounts" element={<Accounts />} />
            <Route path="/evaluation" element={<ModelComparison />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}

export default App;
