import test from 'node:test';
import assert from 'node:assert/strict';
import { offerSource, sourceTool } from '../internal/managed/assets/licensing.mjs';

function fixture() {
  class Server { setRequestHandler(schema, handler) { this.handlers.set(schema.shape.method.value,handler); } constructor(){this.handlers=new Map();} }
  const info={license:'AGPL-3.0-only',source:'https://example.invalid/tree/commit',attribution:'Xpotato.net / Xpotato1024'};
  offerSource(Server, info);
  const server=new Server();
  const set=(method,handler)=>server.setRequestHandler({shape:{method:{value:method}}},handler);
  return {server,set,info};
}

test('initialize retains server details and prominently offers source without stream writes',async()=>{
  const f=fixture(), capabilities={tools:{}};
  f.set('initialize',async()=>({instructions:'Original Commander instructions',capabilities}));
  const result=await f.server.handlers.get('initialize')({});
  assert.equal(result.capabilities,capabilities);
  assert.match(result.instructions,/Original Commander instructions/);
  assert.match(result.instructions,/Xpotato.net \/ Xpotato1024/);
  assert.ok(result.instructions.includes(sourceTool));
});
test('source tool is read-only; existing tools, pagination and results stay intact',async()=>{
  const f=fixture(), original={name:'read_file'};
  f.set('tools/list',async()=>({tools:[original],nextCursor:'next'}));
  const result=await f.server.handlers.get('tools/list')({});
  assert.equal(result.tools[0],original);
  assert.equal(result.nextCursor,'next');
  assert.equal(result.tools[1].annotations.readOnlyHint,true);
  const privateResult={content:[{type:'text',text:'PRIVATE_RESULT'}]};
  f.set('tools/call',async()=>privateResult);
  assert.equal(await f.server.handlers.get('tools/call')({params:{name:'read_file'}}),privateResult);
  const source=await f.server.handlers.get('tools/call')({params:{name:sourceTool,arguments:{path:'PRIVATE_PATH',key:'PRIVATE_KEY'}}});
  assert.deepEqual(JSON.parse(source.content[0].text),f.info);
  assert.doesNotMatch(source.content[0].text,/PRIVATE_/);
});
test('unrelated methods and handler exceptions remain unchanged',async()=>{
  const f=fixture(), result={ok:true}, error=new Error('synthetic');
  f.set('ping',async()=>result);
  assert.equal(await f.server.handlers.get('ping')({}),result);
  f.set('tools/list',async()=>{throw error;});
  await assert.rejects(f.server.handlers.get('tools/list')({}),e=>e===error);
  f.set('tools/call',async()=>{throw error;});
  await assert.rejects(async()=>f.server.handlers.get('tools/call')({params:{name:'original'}}),e=>e===error);
});
test('subsequent pages preserve the original list without duplicating the source tool',async()=>{
  const f=fixture(), page={tools:[{name:'second_page_tool'}],nextCursor:'third'};
  f.set('tools/list',async()=>page);
  assert.equal(await f.server.handlers.get('tools/list')({params:{cursor:'second'}}),page);
});
