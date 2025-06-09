import http from 'http';
import url from 'url';

const PORT = Number(process.env.PORT || 4002);

// Mock organization data
const organizationData = {
  organization: {
    Id: 'o-example123456',
    Arn: 'arn:aws:organizations::123456789012:organization/o-example123456',
    FeatureSet: 'ALL',
    MasterAccountArn: 'arn:aws:organizations::123456789012:account/o-example123456/123456789012',
    MasterAccountId: '123456789012',
    MasterAccountEmail: 'admin@example.com'
  },
  accounts: [
    {
      Id: '123456789012',
      Arn: 'arn:aws:organizations::123456789012:account/o-example123456/123456789012',
      Name: 'Master Account',
      Email: 'admin@example.com',
      Status: 'ACTIVE',
      JoinedMethod: 'INVITED',
      JoinedTimestamp: '2020-01-01T00:00:00Z'
    },
    {
      Id: '123456789013',
      Arn: 'arn:aws:organizations::123456789012:account/o-example123456/123456789013',
      Name: 'Development Account',
      Email: 'dev@example.com',
      Status: 'ACTIVE',
      JoinedMethod: 'CREATED',
      JoinedTimestamp: '2020-02-01T00:00:00Z'
    }
  ]
};

function handleDescribeOrganization(req, res) {
  console.log('[Organizations Mock] DescribeOrganization request');
  
  const response = {
    Organization: organizationData.organization
  };
  
  res.writeHead(200, { 
    'Content-Type': 'application/x-amz-json-1.1',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(JSON.stringify(response));
}

function handleListAccounts(req, res) {
  console.log('[Organizations Mock] ListAccounts request');
  
  const response = {
    Accounts: organizationData.accounts
  };
  
  res.writeHead(200, { 
    'Content-Type': 'application/x-amz-json-1.1',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(JSON.stringify(response));
}

function handleDescribeAccount(req, res, body) {
  console.log('[Organizations Mock] DescribeAccount request');
  
  let payload;
  try {
    payload = JSON.parse(body);
  } catch (e) {
    payload = {};
  }
  
  const accountId = payload.AccountId;
  const account = organizationData.accounts.find(acc => acc.Id === accountId);
  
  if (!account) {
    res.writeHead(400, { 
      'Content-Type': 'application/x-amz-json-1.1',
      'x-amzn-RequestId': generateRequestId()
    });
    res.end(JSON.stringify({
      __type: 'AccountNotFoundException',
      message: `Account ${accountId} not found`
    }));
    return;
  }
  
  const response = {
    Account: account
  };
  
  res.writeHead(200, { 
    'Content-Type': 'application/x-amz-json-1.1',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(JSON.stringify(response));
}

function handleListRoots(req, res) {
  console.log('[Organizations Mock] ListRoots request');
  
  const response = {
    Roots: [
      {
        Id: 'r-exampleroot',
        Arn: 'arn:aws:organizations::123456789012:root/o-example123456/r-exampleroot',
        Name: 'Root',
        PolicyTypes: [
          {
            Type: 'SERVICE_CONTROL_POLICY',
            Status: 'ENABLED'
          }
        ]
      }
    ]
  };
  
  res.writeHead(200, { 
    'Content-Type': 'application/x-amz-json-1.1',
    'x-amzn-RequestId': generateRequestId()
  });
  res.end(JSON.stringify(response));
}

function handleHealth(req, res) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({
    service: 'aws-organizations-mock',
    status: 'healthy',
    endpoints: ['DescribeOrganization', 'ListAccounts', 'DescribeAccount', 'ListRoots'],
    organizationId: organizationData.organization.Id,
    accountCount: organizationData.accounts.length,
    timestamp: new Date().toISOString()
  }));
}

function generateRequestId() {
  return 'mock-' + Math.random().toString(36).substr(2, 9);
}

const server = http.createServer((req, res) => {
  const pathname = url.parse(req.url).pathname;
  const target = req.headers['x-amz-target'];
  
  // Enable CORS
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization, x-amz-target');
  
  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }
  
  console.log(`[Organizations Mock] ${req.method} ${pathname} (target: ${target})`);
  
  if (pathname === '/health') {
    handleHealth(req, res);
    return;
  }
  
  // Handle Organizations requests
  if (req.method === 'POST') {
    let body = '';
    req.on('data', chunk => {
      body += chunk.toString();
    });
    
    req.on('end', () => {
      console.log(`[Organizations Mock] Target: ${target}`);
      
      switch (target) {
        case 'AWSOrganizationsV20161128.DescribeOrganization':
          handleDescribeOrganization(req, res);
          break;
        case 'AWSOrganizationsV20161128.ListAccounts':
          handleListAccounts(req, res);
          break;
        case 'AWSOrganizationsV20161128.DescribeAccount':
          handleDescribeAccount(req, res, body);
          break;
        case 'AWSOrganizationsV20161128.ListRoots':
          handleListRoots(req, res);
          break;
        default:
          res.writeHead(400, { 
            'Content-Type': 'application/x-amz-json-1.1',
            'x-amzn-RequestId': generateRequestId()
          });
          res.end(JSON.stringify({
            __type: 'InvalidParameterException',
            message: `Unknown operation: ${target}`
          }));
      }
    });
  } else {
    res.writeHead(405, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Method Not Allowed' }));
  }
});

server.listen(PORT, () => {
  console.log(`🚀 AWS Organizations Mock Service running on port ${PORT}`);
  console.log(`   Organization ID: ${organizationData.organization.Id}`);
  console.log(`   Accounts: ${organizationData.accounts.length}`);
});
