import React, { useEffect, ReactNode, useState } from "react";
import "./assets/css/main.css";
import { BrowserRouter, Routes, Route, Link } from "react-router-dom";
import Home from "./pages/Home/Home";
import "./assets/css/main.css";
import User from "./pages/User/User";
import MasterList from "./pages/Master/MasterList";
import MasterImport from "./pages/Master/MasterImport";

// App
const App = () => {
  const [sessionId, setSessionId] = useState("");

  const handleSetSessionId = (sessId: string) => {
    setSessionId(sessId);
  };

  // initialize master data
  useEffect(() => {
    localStorage.setItem("master-version", "1");
    const sessId = localStorage.getItem("admin-session");
    setSessionId(sessId === null ? "" : sessId);
  }, []);

  return (
    <div>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Home sessionId={sessionId} handleSetSessionId={handleSetSessionId} />} />
          <Route path="/user" element={<User sessionId={sessionId} handleSetSessionId={handleSetSessionId} />} />
          <Route
            path="/master/list"
            element={<MasterList sessionId={sessionId} handleSetSessionId={handleSetSessionId} />}
          />
          <Route
            path="/master/import"
            element={<MasterImport sessionId={sessionId} handleSetSessionId={handleSetSessionId} />}
          />
        </Routes>
      </BrowserRouter>
    </div>
  );
};

export default App;
