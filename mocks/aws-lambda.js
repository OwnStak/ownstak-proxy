import http from 'http';
import url from 'url';
import fs from 'fs';
import path from 'path';
import mime from 'mime';
import { fileURLToPath } from 'url';
import { dirname } from 'path';
import { readFile } from 'fs/promises'

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const PORT = Number(process.env.PORT || 4001);

// Lambda functions defined as callbacks that accept event and return response
const functions = {
  'ownstak-project-prod': (_event) => {
    return {
      statusCode: 200,
      headers: { 'Content-Type': 'text/html' },
      body: `
        <html>
          <body>
            <h1>Hello from the test lambda function "ownstak-project-prod"!</h1>
          </body>
        </html>
      `
    };
  },
  'default': async (event) => {
    const url = new URL(`${event.rawPath}?${event.rawQueryString}`, 'http://example.com');
    
    if (url.pathname === '/error') {
      const errorType = url.searchParams.get('type') || 'Error';
      const message = url.searchParams.get('message') || 'This is a test error from the mock lambda function';
      throw new LambdaError(message, { errorType });
    }

    // Serve files form ./static directory
    if (url.pathname.startsWith('/static/')) {
      const fileName = url.pathname.split('/').pop();
      const fileExtension = path.extname(fileName).split('.')[1];
      const filePath = path.join(__dirname, 'static', fileName);
      if (fs.existsSync(filePath)) {
        const fileContent = await readFile(filePath, 'base64');
        return {
          statusCode: 200,
          headers: { 'Content-Type': mime.getType(fileExtension) },
          body: fileContent.toString('base64'),
          isBase64Encoded: true
        };
      }else{
        return {
          statusCode: 404,
          headers: { 'Content-Type': 'text/html' },
          body: 'File not found'
        };
      }
    }
    
    if (url.pathname === '/redirect') {
      const location = url.searchParams.get('location') || '/destination';
      const statusCode = Number(url.searchParams.get('statusCode')) || 302;
      const followRedirect = url.searchParams.get('followRedirect') || 'true';
      return {
        statusCode: statusCode,
        headers: { 
          'Location': location ,
          'X-Own-Follow-Redirect': followRedirect,

        }
      };
    }

    return {
      statusCode: 200,
      headers: { 
        'Content-Type': 'application/json',
        'x-custom-header': 'custom-value',
      },
      multiValueHeaders: {
        'x-custom-multi-value-header': ['custom-value-1', 'custom-value-2']
      },
      body: JSON.stringify({ 
        message: 'Hello from the default lambda function!',
        description: 'This is echo response from the mock lambda function. See ./mocks/aws-lambda.js for more details.',
        event
      })
    };
  }
};

// Helper function to parse function name from ARN
function parseFunctionName(functionName) {
  // First decode the URL encoded function name
  const decodedFunctionName = decodeURIComponent(functionName);
  console.log(`[Lambda Mock] Decoded function name: ${decodedFunctionName}`);
  
  // Handle ARN format: arn:aws:lambda:region:account:function:functionName:alias
  if (decodedFunctionName.includes(':')) {
    const arnParts = decodedFunctionName.split(':');
    if (arnParts.length >= 7 && arnParts[5] === 'function') {
      return arnParts[6]; // function name is at index 6
    }
    return arnParts[arnParts.length - 1];
  }
  return decodedFunctionName;
}

// Handle Invoke requests
async function handleInvoke(req, res) {
  const pathname = url.parse(req.url).pathname;
  const pathParts = pathname.split('/');
  
  // Extract function name from path: /2015-03-31/functions/:functionName/invocations
  if (pathParts.length < 5 || pathParts[4] !== 'invocations') {
    res.writeHead(400, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Invalid request path' }));
    return;
  }
  
  const functionName = parseFunctionName(pathParts[3]);
  console.log(`[Lambda Mock] Invoke request: ${functionName}`);
  
  // Read request body
  let body = '';
  req.on('data', chunk => {
    body += chunk.toString();
  });
  
  req.on('end', async () => {
    let payload;
    try {
      payload = JSON.parse(body);
    } catch (e) {
      payload = { rawPath: '/' };
    }

    // Handle ownstak-404 error
    if (functionName === 'ownstak-404') {
      res.writeHead(404, { 
        'Content-Type': 'application/json',
        'x-amzn-RequestId': generateRequestId(),
        'x-amz-function-error': 'ResourceNotFoundException'
      });
      return res.end(JSON.stringify({
        "__type":  "ResourceNotFoundException",
        "message": "The resource you requested does not exist."
      }));
    }

    // Get the function callback or use default
    const functionCallback = functions[functionName] || functions['default'];
    
    try {
      // Call the function with the payload
      const response = await functionCallback(payload);
      
      console.log(`[Lambda Mock] Returning response: ${response.statusCode}`);
      res.writeHead(200, { 
        'Content-Type': 'application/json',
        'x-amzn-RequestId': generateRequestId()
      });
      res.end(JSON.stringify(response));
    } catch (error) {
      console.error(`[Lambda Mock] Returning lambda error response: ${error.message}`);
      res.writeHead(200, { 
        'Content-Type': 'application/json',
        'x-amzn-RequestId': generateRequestId(),
        'x-amz-function-error': 'Unhandled'
      });
      
      // AWS Lambda error response format
      const errorPayload = {
        errorType: error.errorType || 'Runtime.HandlerError',
        errorMessage: error.message || 'Unknown error',
        trace: error.stack ? error.stack.split('\n') : [],
      };
      
      res.end(JSON.stringify(errorPayload));
    }
  });
}

function handleHealth(req, res) {
  res.writeHead(200, { 'Content-Type': 'application/json' });
  res.end(JSON.stringify({
    service: 'aws-lambda-mock',
    status: 'healthy',
    specificFunctions: Object.keys(functions),
    defaultHandler: 'available for any function name',
    timestamp: new Date().toISOString()
  }));
}

function generateRequestId() {
  return 'mock-' + Math.random().toString(36).substr(2, 9);
}

const server = http.createServer((req, res) => {
  const pathname = url.parse(req.url).pathname;
  
  // Enable CORS
  res.setHeader('Access-Control-Allow-Origin', '*');
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS');
  res.setHeader('Access-Control-Allow-Headers', 'Content-Type, Authorization');
  
  if (req.method === 'OPTIONS') {
    res.writeHead(204);
    res.end();
    return;
  }
  
  console.log(`[Lambda Mock] ${req.method} ${pathname}`);
  
  if (pathname === '/health') {
    handleHealth(req, res);
  } else if (pathname.includes('/invocations')){
    handleInvoke(req, res);
  } else {
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Not Found' }));
  }
});

server.listen(PORT, () => {
  console.log(`🚀 AWS Lambda Mock Service running on port ${PORT}`);
  console.log(`   Functions: ${Object.keys(functions).join(', ')}`);
});


class LambdaError extends Error {
  constructor(message, { errorType }) {
    super(message);
    this.errorType = errorType;
  }
}
