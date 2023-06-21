const express = require('express');
const { S3Client, PutObjectCommand, GetObjectCommand } = require("@aws-sdk/client-s3");
const fs = require('fs').promises;
const { createReadStream } = require('fs');

const app = express();
app.use(express.json());

const REGION_NAME = 'us-west-2';
const BUCKET_NAME = 'my-sample-bucket';
const FILE_NAME   = 'file1.json';

const s3 = new S3Client({
  region: REGION_NAME,
  credentials: {
    accessKeyId: process.env.AWS_ACCESS_KEY,
    secretAccessKey: process.env.AWS_SECRET_KEY
  }
});

app.get('/', (req, res) => {
    res.json({ status: 'ok' });
});

app.post('/upload', async (req, res) => {
    let data = req.body;

    try {
        await fs.writeFile(FILE_NAME, JSON.stringify(data));
        const fileStream = createReadStream(FILE_NAME);

        const uploadParams = {
            Bucket: BUCKET_NAME,
            Key: FILE_NAME,
            Body: fileStream
        };
        await s3.send(new PutObjectCommand(uploadParams));
        res.json({ status: 'ok' });
    } catch (err) {
        res.status(500).json({ error: err.message });
    }
});

app.get('/download', async (req, res) => {
    const downloadParams = {
        Bucket: BUCKET_NAME,
        Key: FILE_NAME,
    };

    try {
        const { Body } = await s3.send(new GetObjectCommand(downloadParams));
        let responseBody = '';
        for await (let chunk of Body) {
            responseBody += chunk;
        }
        res.json(JSON.parse(responseBody));
    } catch (err) {
        if (err.name === 'NoSuchKey') {
            res.status(404).json({ error: 'File not found in S3 bucket' });
        } else {
            res.status(500).json({ error: 'Unknown error' });
        }
    }
});

app.listen(3000, () => console.log('App listening on port 3000'));
