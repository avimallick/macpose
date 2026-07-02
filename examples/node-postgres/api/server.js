const express = require("express");

const app = express();
const port = process.env.PORT || 3000;

function serviceHost(name) {
  const key = `MACPOSE_SERVICE_${name.toUpperCase()}_HOST`;
  return process.env[key] || name;
}

app.get("/health", (_req, res) => {
  res.json({ status: "ok", database_host: serviceHost("db") });
});

app.get("/", (_req, res) => {
  res.json({ message: "Hello from Macpose + Node" });
});

app.listen(port, () => {
  console.log(`listening on ${port}`);
});
