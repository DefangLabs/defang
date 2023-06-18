const express = require('express');
const fetch = require('node-fetch');
const app = express();

app.get('/', (req, res) => {
  res.json({ status: "ok" });
});

app.get('/rates', async (req, res) => {
  const url = "https://api.fiscaldata.treasury.gov/services/api/fiscal_service/v2/accounting/od/avg_interest_rates?page[number]=1&page[size]=10";
  
  try {
    const response = await fetch(url, {
      headers: {
        "Content-Type": "application/json",
      }
    });
    if (!response.ok) {
      res.json({ status: "error" });
      return;
    }
    const data = await response.json();
    res.json(data);
  } catch (err) {
    console.error(err);
    res.json({ status: "error" });
  }
});

app.listen(3000, '0.0.0.0', () => {
  console.log('Server running on http://0.0.0.0:3000')
});
