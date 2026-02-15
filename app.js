import express from "express";
import cors from "cors";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();
app.use(cors());
app.use(express.json());

const CODES_FILE = path.join(__dirname, "codes.json");

function readCodes() {
  return JSON.parse(fs.readFileSync(CODES_FILE, "utf-8"));
}

function writeCodes(codes) {
  fs.writeFileSync(CODES_FILE, JSON.stringify(codes, null, 2), "utf-8");
}

app.post("/api/activate", (req, res) => {
  const { code, machineId } = req.body;
  if (!code || typeof code !== "string") {
    return res.status(400).json({ success: false, message: "缺少激活码" });
  }
  if (!machineId || typeof machineId !== "string") {
    return res.status(400).json({ success: false, message: "缺少机器码" });
  }

  const codes = readCodes();
  const trimmed = code.trim().toUpperCase();
  const entry = codes.find((c) => c.code === trimmed);

  if (!entry) {
    return res.json({ success: false, message: "激活码无效" });
  }

  if (entry.active && entry.machineId && entry.machineId !== machineId) {
    return res.json({ success: false, message: "该激活码已被其他设备使用" });
  }

  if (entry.active && entry.machineId === machineId) {
    return res.json({ success: true, message: "已激活" });
  }

  entry.active = true;
  entry.machineId = machineId;
  entry.activatedAt = new Date().toISOString();
  writeCodes(codes);

  return res.json({ success: true, message: "激活成功" });
});

app.get("/api/health", (req, res) => {
  res.json({ status: "ok" });
});

const PORT = process.env.PORT || 3000;
app.listen(PORT, () => {
  console.log(`Activation server running on port ${PORT}`);
});
