import http from 'http';
import url from 'url';

const PORT = Number(process.env.PORT || 4003);

function handleGetCallerIdentity(req, res) {
  console.log('[STS Mock] GetCallerIdentity request');

  const xmlResponse = `<?xml version="1.0" encoding="UTF-8"?>
<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
    <GetCallerIdentityResult>
        <Arn>arn:aws:iam::123456789012:root</Arn>
        <UserId>123456789012</UserId>
        <Account>123456789012</Account>
    </GetCallerIdentityResult>
    <ResponseMetadata>
        <RequestId>mock-${Math.random().toString(36).substr(2, 9)}</RequestId>
    </ResponseMetadata>
</GetCallerIdentityResponse>`;

  res.writeHead(200, {
    'Content-Type': 'text/xml',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(xmlResponse);
}

function handleAssumeRole(req, res) {
  console.log('[STS Mock] AssumeRole request');

  const xmlResponse = `<?xml version="1.0" encoding="UTF-8"?>
<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
    <AssumeRoleResult>
        <Credentials>
            <AccessKeyId>ASIAMOCKACCESKEYID</AccessKeyId>
            <SecretAccessKey>mocksecretaccesskey</SecretAccessKey>
            <SessionToken>mocktoken</SessionToken>
            <Expiration>2024-12-31T23:59:59Z</Expiration>
        </Credentials>
        <AssumedRoleUser>
            <AssumedRoleId>AROAMOCKROLEID:mock-session</AssumedRoleId>
            <Arn>arn:aws:sts::123456789012:assumed-role/MockRole/mock-session</Arn>
        </AssumedRoleUser>
    </AssumeRoleResult>
    <ResponseMetadata>
        <RequestId>mock-${Math.random().toString(36).substr(2, 9)}</RequestId>
    </ResponseMetadata>
</AssumeRoleResponse>`;

  res.writeHead(200, {
    'Content-Type': 'text/xml',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(xmlResponse);
}

function handleHealth(req, res) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({
    service: 'aws-sts-mock',
    status: 'healthy',
    endpoints: ['GetCallerIdentity', 'AssumeRole'],
    timestamp: new Date().toISOString()
  }));
}

function generateRequestId() {
  return 'mock-' + Math.random().toString(36).substr(2, 9);
}

// EC2 Instance Metadata Service handlers
function handleIMDSTokenRequest(req, res) {
  console.log('[IMDS Mock] Token request');
  // Return a mock IMDSv2 token
  res.writeHead(200, {
    'Content-Type': 'text/plain',
  });
  res.end('mockimdstoken');
}

function handleIMDSRoleRequest(req, res) {
  console.log('[IMDS Mock] Role name request');
  res.writeHead(200, {
    'Content-Type': 'text/plain',
  });
  res.end('mock-role');
}

function handleIMDSCredentialsRequest(req, res) {
  console.log('[IMDS Mock] Credentials request');
  const credentials = {
    Code: 'Success',
    LastUpdated: new Date().toISOString(),
    Type: 'AWS-HMAC',
    AccessKeyId: 'ASIAMOCKACCESKEYID',
    SecretAccessKey: 'mocksecretaccesskey',
    Token: 'mocktoken',
    Expiration: '2024-12-31T23:59:59Z'
  };

  res.writeHead(200, {
    'Content-Type': 'application/json',
  });
  res.end(JSON.stringify(credentials));
}

// Combined server that handles both STS and IMDS requests
const server = http.createServer((req, res) => {
  const pathname = url.parse(req.url).pathname;
  const query = url.parse(req.url, true).query;

  // Enable CORS
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, PUT, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, x-amz-target, x-aws-ec2-metadata-token-ttl-seconds');

  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }

  console.log(`[Mock] ${req.method} ${pathname}`);

  // Health check
  if (pathname === '/health') {
    handleHealth(req, res);
    return;
  }

  // IMDS endpoints
  if (pathname === '/latest/api/token' && req.method === 'PUT') {
    handleIMDSTokenRequest(req, res);
    return;
  }

  if (pathname === '/latest/meta-data/iam/security-credentials/' && req.method === 'GET') {
    handleIMDSRoleRequest(req, res);
    return;
  }

  if (pathname.match(/^\/latest\/meta-data\/iam\/security-credentials\//) && req.method === 'GET') {
    handleIMDSCredentialsRequest(req, res);
    return;
  }

  // STS requests
  if (req.method === 'POST') {
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });

    req.on('end', () => {
      // Parse form data
      const params = new URLSearchParams(body);
      const action = params.get('Action') || query.Action;

      console.log(`[STS] Action: ${action}`);

      switch (action) {
        case 'GetCallerIdentity':
          handleGetCallerIdentity(req, res);
          break;
        case 'AssumeRole':
          handleAssumeRole(req, res);
          break;
        default:
          res.writeHead(400, { 'Content-Type': 'text/xml' });
          res.end(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
    <Error>
        <Type>Sender</Type>
        <Code>InvalidAction</Code>
        <Message>Invalid action: ${action}</Message>
    </Error>
    <RequestId>mock-${Math.random().toString(36).substr(2, 9)}</RequestId>
</ErrorResponse>`);
      }
    });
    return;
  }

  // Default 404
  res.writeHead(404, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({ message: 'Not Found' }));
});

server.listen(PORT, '127.0.0.1', () => {
  console.log(`🚀 AWS STS & IMDS Mock Service running on http://127.0.0.1:${PORT}`);
  console.log(`   - STS endpoints: GetCallerIdentity, AssumeRole`);
  console.log(`   - IMDS endpoints: /latest/api/token, /latest/meta-data/iam/security-credentials/*`);
});
