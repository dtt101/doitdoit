import { env } from 'cloudflare:workers';
import { expect, it } from 'vitest';
import { RustTasks, request } from './client.js';
import { signIn } from './auth-helper.js';
async function client(scopes=['tasks:read','tasks:write']) {
  await env.DB.prepare("INSERT INTO owners(id,timezone) VALUES('github-12345','Europe/London') ON CONFLICT DO NOTHING").run();
  return new RustTasks(env,{ownerID:'github-12345',clientID:'lifecycle-test',scopes});
}
const mutation=revision=>({operation_id:crypto.randomUUID(),expected_revision:revision});
const total=async table=>env.DB.prepare(`SELECT count(*) AS n FROM ${table}`).first('n');
it('soft deletes/restores with retained dates, completion, notes, history and original retry acknowledgements', async()=>{
  const c=await client();
  const created=await c.create({operation_id:crypto.randomUUID(),title:'Preserve',notes:'☕\n  note',scheduled_date:'2020-02-29'});
  const completed=await c.update(created.id,{...mutation(1),changes:{status:'completed'}});
  const command=mutation(2);
  const deleted=(await c.call('/v1/tasks/'+created.id,'DELETE',command)).task;
  expect(deleted).toMatchObject({...completed,revision:3,deleted_at:expect.any(String),updated_at:expect.any(String)});
  expect(await c.list()).toEqual([]);
  expect(await c.get(created.id)).toEqual(deleted);
  await expect(c.update(created.id,{...mutation(3),changes:{title:'bad'}})).rejects.toMatchObject({code:'revision_conflict',current:deleted});
  const restored=(await c.call('/v1/tasks/'+created.id+'/restore','POST',mutation(3))).task;
  expect(restored).toMatchObject({revision:4,status:'completed',scheduled_date:'2020-02-29',deleted_at:null,notes:created.notes,completed_at:completed.completed_at});
  expect((await c.call('/v1/tasks/'+created.id,'DELETE',command)).task).toEqual(deleted);
  const page=await c.call('/v1/tasks/'+created.id+'/history?limit=2');
  expect(page.events.map(e=>e.action)).toEqual(['create','update']);
  expect(page.events[0]).toMatchObject({before:null,after:created});
  const rest=await c.call('/v1/tasks/'+created.id+'/history?after='+page.next_cursor+'&limit=2');
  expect(rest.events.map(e=>e.action)).toEqual(['delete','restore']);
  expect(rest.events[0]).toMatchObject({before:completed,after:deleted});
  expect(rest.next_cursor).toBeNull();
  expect(await total('operation_receipts')).toBe(4);
});
it('races delete against an edit and rolls back unsuccessful lifecycle commands',async()=>{
  const c=await client();const task=await c.create({operation_id:crypto.randomUUID(),title:'Race'});
  const r=await Promise.allSettled([
    c.call('/v1/tasks/'+task.id,'DELETE',mutation(1)),
    c.update(task.id,{...mutation(1),changes:{scheduled_date:'2030-01-02'}}),
  ]);
  expect(r.filter(x=>x.status==='fulfilled')).toHaveLength(1);
  expect(r.find(x=>x.status==='rejected').reason.code).toBe('revision_conflict');
  expect(await total('task_events')).toBe(2);expect(await total('operation_receipts')).toBe(2);
  const current=await c.get(task.id);
  await env.DB.prepare("CREATE TRIGGER fail_receipt BEFORE UPDATE ON operation_receipts BEGIN SELECT RAISE(ABORT,'injected'); END").run();
  const path='/v1/tasks/'+task.id+(current.deleted_at?'/restore':'');
  await expect(c.call(path,current.deleted_at?'POST':'DELETE',mutation(2))).rejects.toMatchObject({status:500});
  expect(await c.get(task.id)).toEqual(current);
  expect(await total('task_events')).toBe(2);
});
it('exposes lifecycle and history through MCP with the same revision and scope rules',async()=>{
  const login=await signIn();
  async function tool(name,args,token=login.token){
    const r=await request('/mcp',{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json',Accept:'application/json, text/event-stream'},body:JSON.stringify({jsonrpc:'2.0',id:1,method:'tools/call',params:{name,arguments:args}})});
    if(r.status!==200)return {status:r.status};
    return (await r.json()).result;
  }
  const created=(await tool('create_task',{operation_id:crypto.randomUUID(),title:'MCP lifecycle'})).structuredContent.task;
  const deleted=await tool('delete_task',{id:created.id,...mutation(1)});
  expect(deleted.isError).toBe(false);expect(deleted.structuredContent.task.deleted_at).not.toBeNull();
  const stale=await tool('restore_task',{id:created.id,...mutation(1)});
  expect(stale.isError).toBe(true);expect(stale.structuredContent.error.code).toBe('revision_conflict');
  expect((await tool('restore_task',{id:created.id,...mutation(2)})).structuredContent.task.deleted_at).toBeNull();
  expect((await tool('task_history',{id:created.id})).structuredContent.events).toHaveLength(3);
  const readonly=await signIn('tasks:read');
  expect(await tool('delete_task',{id:created.id,...mutation(3)},readonly.token)).toEqual({status:403});
});
