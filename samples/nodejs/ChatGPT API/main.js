const express = require('express');
const axios = require('axios');
const bodyParser = require('body-parser');

const app = express();
app.use(bodyParser.text());

app.get('/', (req, res) => {
  res.json({status: 'ok'});
});

app.post('/prompt', async (req, res) => {
  const promptText = req.body;
  
  const messages = [
    // {role: 'system', content: 'You are an experienced software engineer.'},
    {role: 'user', content: promptText},
  ];

  const apiKey = process.env.OPENAI_KEY;
  const url = 'https://api.openai.com/v1/chat/completions';

  const postBody = {
    model: 'gpt-3.5-turbo',
    messages: messages,
  };

  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${apiKey}`
  };

  try {
    const response = await axios.post(url, postBody, {headers: headers});
    res.json({status: 'ok', response: response.data});
  } catch (error) {
    console.error(error);
    res.status(500).json({status: 'error', message: error.message});
  }
});

app.listen(3000, () => {
  console.log('Server running on port 3000');
});
