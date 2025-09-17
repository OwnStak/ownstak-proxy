import http from 'http';
import url from 'url';
import fs from 'fs';
import path from 'path';
import mime from 'mime';
import cluster from 'cluster';
import os from 'os';
import { fileURLToPath } from 'url';
import { dirname } from 'path';
import { readFile } from 'fs/promises'
import { EventStreamCodec } from '@smithy/eventstream-codec';
import { fromUtf8, toUtf8 } from "@smithy/util-utf8";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const PORT = Number(process.env.PORT || 4001);
const WORKERS = Number(process.env.WORKERS || 8);
const HANDLER_STREAMING = Symbol.for('aws.lambda.runtime.handler.streaming');

// 8 null bytes indicates the end of response headers part and start of the body in streaming mode.
// This special marker cannot appear anywhere in the res headers.
// e.g: "\x00\x00\x00\x00\x00\x00\x00\x00"
const STREAMING_BODY_DELIMITER = '\x00'.repeat(8);

// Fake global awsLambda object that exposes helper functions
// Implemented based on real AWS Lambda Runtime Interface
// See: https://github.com/aws/aws-lambda-nodejs-runtime-interface-client/blob/main/src/types/awslambda.d.ts
// See: https://github.com/aws/aws-lambda-nodejs-runtime-interface-client/blob/main/src/UserFunction.js#L205
const awsLambda = {
  streamifyResponse: (handler) => {
    const wrappedHandler = async (event, responseStream, context) => {
      return handler(event, responseStream, context);
    }
    // Mark the handler as streaming by attaching symbol to function object
    wrappedHandler[HANDLER_STREAMING] = true;
    return wrappedHandler;
  }
}

// Lambda functions defined as callbacks that accept event, context 
// and optionally response stream if it's enabled using awsLambda.streamifyResponse.
// All below lambda handlers should match the AWS Lambda Runtime Interface and work with real AWS Lambda.
const functions = {
  'ownstak-project-prod': awsLambda.streamifyResponse((_event, responseStream, _context) => {
    const body = `
      <html>
        <body>
          <h1>Hello from the test lambda function "ownstak-project-prod"!</h1>
        </body>
      </html>
    `
    responseStream.write(JSON.stringify({
      statusCode: 200,
      headers: { 'Content-Type': 'text/html' },
      body: Buffer.from(body, 'utf8').toString('base64'),
      isBase64Encoded: true
    }));
  }),
  'ownstak-project-prod-streaming': awsLambda.streamifyResponse(async (_event, responseStream, _context) => {
    const body = `
      <html>
        <body>
          <h1>Hello from the test lambda function "ownstak-project-prod-streaming"!</h1>
        </body>
      </html>
    `
    responseStream.write(JSON.stringify({
      statusCode: 200,
      headers: { 'Content-Type': 'text/html' },
      isBase64Encoded: false
    }));
    responseStream.write(STREAMING_BODY_DELIMITER ) // This is special marker to indicate the start of the body
    responseStream.write(body)
    responseStream.end()
  }),
  'ownstak-project-prod-buffered': async (_event, _context) => {
    const body = `
      <html>
        <body>
          <h1>Hello from the test lambda function "ownstak-project-prod-buffered"!</h1>
        </body>
      </html>
    `
    return {
      statusCode: 200,
      headers: { 'Content-Type': 'text/html' },
      body: Buffer.from(body, 'utf8').toString('base64'),
      isBase64Encoded: true
    }
  },
  'ownstak-streaming': awsLambda.streamifyResponse(async (_event, responseStream, _context) => {
    responseStream.write(JSON.stringify({
      statusCode: 200,
      headers: { 'Content-Type': 'text/html; charset=utf-8' },
    }));
    responseStream.write(STREAMING_BODY_DELIMITER)
    for (let i = 0; i < 20; i++) {
      responseStream.write(`Number ${i}</br>\n`)
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    responseStream.end()
  }),
  'ownstak-streaming-error1': awsLambda.streamifyResponse(async (_event, responseStream, _context) => {
    throw new Error('This is error before the headers are sent. It should return JSON/HTML response with error details and 5xx status code');
  }),
  'ownstak-streaming-error2': awsLambda.streamifyResponse(async (_event, responseStream, _context) => {
    responseStream.write(JSON.stringify({
      statusCode: 200,
      headers: { 'Content-Type': 'text/html; charset=utf-8' },
    }));
    responseStream.write(STREAMING_BODY_DELIMITER)
    for (let i = 0; i < 20; i++) {
      responseStream.write(`Number ${i}</br>\n`)
      await new Promise(resolve => setTimeout(resolve, 100));
      if(i === 19) {
        throw new Error('This is error in the middle of the response body streaming. The 200 status code was already sent, so it should trigger TCP reset or RST_STREAM based on used HTTP protocol version.');
      }
    }
    responseStream.end()
  }),
  'ownstak-540': (_event) => {
    throw new Error('My custom error from lambda handler. Failed to import next.js from node_modules');
  },
  'ownstak-541': (_event) => {
    throw new LambdaError('Request payload is invalid', { 
      errorType: 'RequestInvalid'
    });
  },
  'ownstak-542': (_event) => {
    throw new LambdaError('Response payload is invalid', { 
      errorType: 'ResponseInvalid'
    });
  },
  'ownstak-543': (_event) => {
    throw new LambdaError('Request payload is too large', { 
      errorType: 'RequestTooLarge'
    });
  },
  'ownstak-544': (_event) => {
    throw new LambdaError('Response payload is too large', { 
      errorType: 'ResponseSizeTooLarge'
    });
  },
  'ownstak-545': (_event) => {
    throw new LambdaError('Task timed out after 20.0 seconds', { 
      errorType: 'Sandbox.Timeout'
    });
  },
  'ownstak-546': (_event) => {
    throw new LambdaError('Too many requests', { 
      errorType: 'TooManyRequests'
    });
  },
  'ownstak-547': (_event) => {
    throw new LambdaError('Task exited with status code 1', { 
      errorType: 'Runtime.ExitError'
    });
  },
  'default': async (event) => {
    const url = new URL(`${event.rawPath}?${event.rawQueryString}`, 'http://example.com');

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

// Create EventStream codec instance with Node.js TextEncoder/TextDecoder
const eventStreamCodec = new EventStreamCodec(toUtf8, fromUtf8);

// Helper function to create proper EventStream messages
function createEventStreamMessage(eventType, payload) {
  const message = {
    headers: {
      ':event-type': { type: 'string', value: eventType },
      ':message-type': { type: 'string', value: 'event' },
      ':content-type': { type: 'string', value: 'application/octet-stream' }
    },
    body: Buffer.from(payload, 'utf8')
  };
  
  return eventStreamCodec.encode(message);
}

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

async function handleInvoke(req, res, withResponseStream = false) {
  const pathname = new URL(req.url, 'http://localhost').pathname;
  const pathParts = pathname.split('/');
  
  // Extract function name from path: 
  // /2015-03-31/functions/:functionName/invocations
  // /2023-07-25/functions/:functionName/response-streaming-invocations
  if (pathParts.length < 5) {
    res.writeHead(400, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Invalid request path for lambda invocation' }));
    return;
  }
  
  const functionName = parseFunctionName(pathParts[3]);
  console.log(`[Lambda Mock] ${withResponseStream ? 'InvokeWithResponseStream' : 'Invoke'} request: ${functionName}`);
  
  // Read request body
  let body = '';
  req.on('data', chunk => {
    body += chunk.toString();
  });
  
  req.on('end', async () => {
    let functionEvent;
    try {
      functionEvent = JSON.parse(body);
    } catch (e) {
      functionEvent = { rawPath: '/' };
    }

    // Simulate not found lambda function
    // This API response error is same for both streaming and buffered invocations
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

    // Helper functions to write API response
    // back to AWS SDK in format it expects
    let headSent = false;
    let ended = false;
    const requestId = generateRequestId();
    const writeHead = (statusCode = 200, headers = {}) => {
      if (ended) return;
      if (headSent) return;
      headSent = true;

      if (withResponseStream) {
        // AWS Lambda InvokeWithResponseStream returns EventStream format
        // with X-Amzn-Remapped-Content-Type: application/json
        res.writeHead(statusCode, { 
          'Content-Type': 'application/vnd.amazon.eventstream',
          'X-Amzn-Remapped-Content-Type': 'application/octet-stream',
          'X-Amzn-RequestId': requestId,
          ...headers
        });
      }else{
        res.writeHead(statusCode, { 
          'Content-Type': 'application/json',
          'x-amzn-RequestId': requestId,
          ...headers
        });
      }
    }
    const write = (data, eventType = 'PayloadChunk') => {
      if (ended) return;
      if (!headSent) writeHead();

      // Split larger data into smaller chunks
      // to simulate AWS EventStream chunking
      const chunkSize = 32 * 1024; // 32 KiB
      const chunks = []
      for(let i = 0; i < data.length; i += chunkSize) {
        chunks.push(data.slice(i, i + chunkSize));
      }
      for(const chunk of chunks) {
        if(withResponseStream) {
          res.write(createEventStreamMessage(eventType, chunk));
        } else {
          res.write(chunk);
        }
      }
    }

    const end = (data = null) => {
      if (ended) return;
      ended = true;
      if (data) write(data);
      res.end();
    }
    
    const writeError = (error) => {
      const errorType = error.errorType || error.name || 'Runtime.HandlerError';
      const errorMessage = error.message || 'Unknown error';
      const errorTrace = error.stack ? error.stack.split('\n') : [];

      // Return buffered error in JSON format that AWS SDK expects
      if(!headSent){
        writeHead(200, { 
          'x-amz-function-error': errorType
        });
      }
      const errorResponse = {
        errorType: errorType,
        errorMessage: errorMessage,
        trace: errorTrace,
      }
      if(withResponseStream) {
        // For streaming invocations, the error response is wrapped into another eventstream envelope
        // object with ErrorCode and ErrorDetails fields.
        // The chunk is sent with event type 'Error'
        write(JSON.stringify({
          ErrorCode: errorResponse.errorType,
          ErrorDetails: JSON.stringify(errorResponse),
        }), 'InvokeComplete');
      } else {
        // For buffered invocations, the error response is returned as it is
        // just like the normal response
        write(JSON.stringify(errorResponse));
      }
    }

    // Get the function handler and create fake objects that lambda function expects
    const functionHandler = functions[functionName] || functions['default'];
    const functionContext = {
      awsRequestId: requestId,
      functionName: functionName,
      functionVersion: '$LATEST',
    }
    const functionResponseStream = {
      write: (data) => write(data),
      end: (data = null) => end(data)
    }

    try {
      // Detect if lambda function handler supports streaming
      // and pass it the optional response stream
      const bufferedResponse = await (async () => {
        if (functionHandler[HANDLER_STREAMING]) {
          // Handler that can directly stream response
          console.log(`[Lambda Mock] Invoking streaming response handler: ${functionName}`);
          return functionHandler(functionEvent, functionResponseStream, functionContext);
        } else {
          // Handler that can only return buffered response
          console.log(`[Lambda Mock] Invoking buffered response handler: ${functionName}`);
          return functionHandler(functionEvent, functionContext);
        }
      })();
      // Return buffered response from fake AWS API if handler returns it.
      // If handler doesn't return anything, it means it streamed the response to response stream.
      if (bufferedResponse){
        write(JSON.stringify(bufferedResponse)) // Write the buffered response
      }else{
        write(JSON.stringify({}), "InvokeComplete"); // End the stream with empty InvokeComplete event
      }
    } catch (error) {
      console.error(`[Lambda Mock] Returning ${withResponseStream ? 'streaming' : 'buffered'} lambda error response: ${error.message}`);
      // Return buffered error in JSON format that AWS SDK expects
      writeError(error);
    }
    end();
  });
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
  
  if (pathname.includes('/invocations')){
    handleInvoke(req, res, false);
  } else if (pathname.includes('/response-streaming-invocations')) {
    handleInvoke(req, res, true);
  } else {
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ message: 'Not Found' }));
  }
});

// Cluster management
if (cluster.isPrimary) {
  console.log(`🚀 AWS Lambda Mock Service starting with ${WORKERS} workers on port ${PORT}`);
  console.log(`   Supported operations:`);
  console.log(`   - Invoke: /2015-03-31/functions/:functionName/invocations`);
  console.log(`   - InvokeWithResponseStream: /2023-07-25/functions/:functionName/response-streaming-invocations`);
  console.log(`   - Functions: ${Object.keys(functions).join(', ')}`);
  
  // Fork workers
  for (let i = 0; i < WORKERS; i++) {
    cluster.fork();
  }
  
  // Handle worker deaths
  cluster.on('exit', (worker, code, signal) => {
    console.log(`❌ Worker ${worker.process.pid} died (${signal || code}). Restarting...`);
    cluster.fork();
  });
  
  // Handle worker online events
  cluster.on('online', (worker) => {
    console.log(`✅ Worker ${worker.process.pid} is online`);
  });
  
} else {
  // Worker process - start the HTTP server
  server.listen(PORT, () => {
    console.log(`⚡ Worker ${process.pid} listening on port ${PORT}`);
  });
  
  // Graceful shutdown for workers
  process.on('SIGTERM', () => {
    console.log(`🛑 Worker ${process.pid} received SIGTERM, shutting down gracefully`);
    server.close(() => {
      process.exit(0);
    });
  });
}


class LambdaError extends Error {
  constructor(message, { errorType }) {
    super(message);
    this.errorType = errorType;
  }
}