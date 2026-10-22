import {env} from 'cloudflare:workers';
import {it,expect} from 'vitest';
import {RustTasks} from './client.js';
async function client(){await env.DB.prepare("INSERT INTO owners(id,timezone) VALUES('github-12345','Europe/London') ON CONFLICT DO NOTHING").run();return new RustTasks(env,{ownerID:'github-12345',clientID:'import-test',scopes:['tasks:read','tasks:write']});}
const fixture=()=>({id:crypto.randomUUID(),title:'Imported',notes:'  ☕\n',status:'completed',scheduled_date:'2020-02-29',position:'0000000000100000i',revision:7,created_at:'2019-01-01T00:00:00Z',updated_at:'2020-01-01T00:00:00Z',completed_at:null,deleted_at:null});
async function begin(c,n,hash='a'.repeat(64)){return (await c.call('/v1/imports','POST',{operation_id:crypto.randomUUID(),source_sha256:hash,expected_count:n})).run;}
it('bounds staging batches with room for authentication under the Free query budget',async()=>{
 const c=await client();const run=await begin(c,26);const tasks=Array.from({length:26},fixture);
 await expect(c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks})).rejects.toMatchObject({status:400});
 expect((await c.call('/v1/imports/'+run.id)).run.staged_count).toBe(0);
 await c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:tasks.slice(0,25)});
 expect((await c.call('/v1/imports/'+run.id)).run.staged_count).toBe(25);
});
it('stages invisibly, resumes identical chunks and atomically publishes exact portable fields once',async()=>{
 const c=await client();const tasks=[fixture(),{...fixture(),title:'Deleted',deleted_at:'2021-01-01T00:00:00Z'}];const run=await begin(c,2);
 await c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:[tasks[0]]});
 expect(await c.list()).toEqual([]);
 await expect(c.call(`/v1/imports/${run.id}/commit`,'POST',{})).rejects.toMatchObject({code:'import_conflict'});
 await c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:[tasks[0]]});
 await expect(c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:[{...tasks[0],notes:'different'}]})).rejects.toMatchObject({code:'import_conflict'});
 await c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:[tasks[1]]});
 const ack=await c.call(`/v1/imports/${run.id}/commit`,'POST',{});expect(ack.imported).toBe(2);
 expect(await c.call(`/v1/imports/${run.id}/commit`,'POST',{})).toEqual(ack);
 expect((await begin(c,2)).id).toBe(run.id);
 const exported=await c.call('/v1/export?limit=100');
 expect(exported.tasks.sort((a,b)=>a.id.localeCompare(b.id))).toEqual(tasks.sort((a,b)=>a.id.localeCompare(b.id)));
 expect(await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n')).toBe(2);
 expect(await env.DB.prepare('SELECT count(*) AS n FROM operation_receipts').first('n')).toBe(1);
 expect((await c.call('/v1/tasks/'+tasks[0].id+'/history')).events[0].action).toBe('import');
});
it('rolls back every task/history/ack/run-state if publication fails, and detects existing task IDs',async()=>{
 const c=await client();const task=fixture();const run=await begin(c,1);
 await c.call(`/v1/imports/${run.id}/tasks`,'POST',{tasks:[task]});
 await env.DB.prepare("CREATE TRIGGER fail_receipt BEFORE UPDATE ON operation_receipts BEGIN SELECT RAISE(ABORT,'injected'); END").run();
 await expect(c.call(`/v1/imports/${run.id}/commit`,'POST',{})).rejects.toMatchObject({status:500});
 expect((await c.call('/v1/imports/'+run.id)).run.committed_at).toBeNull();expect(await c.list()).toEqual([]);
 expect(await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n')).toBe(0);
 await env.DB.prepare('DROP TRIGGER fail_receipt').run();
 await c.call(`/v1/imports/${run.id}/commit`,'POST',{});
 const other=await begin(c,1,'b'.repeat(64));await c.call(`/v1/imports/${other.id}/tasks`,'POST',{tasks:[task]});
 await expect(c.call(`/v1/imports/${other.id}/commit`,'POST',{})).rejects.toMatchObject({code:'import_conflict'});
 expect(await env.DB.prepare('SELECT count(*) AS n FROM tasks').first('n')).toBe(1);
});
