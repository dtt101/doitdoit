import { env } from 'cloudflare:workers';
import { expect,it } from 'vitest';
import { RustTasks } from './client.js';
async function client(){
 await env.DB.prepare("INSERT INTO owners(id,timezone) VALUES('github-12345','Europe/London') ON CONFLICT DO NOTHING").run();
 return new RustTasks(env,{ownerID:'github-12345',clientID:'board-test',scopes:['tasks:read','tasks:write']});
}
const mutation=revision=>({operation_id:crypto.randomUUID(),expected_revision:revision});
const create=(c,title,extra={})=>c.create({operation_id:crypto.randomUUID(),title,...extra});
it('reconstructs a fixed board across edits, deletes and creates and resumes every subsequent change',async()=>{
 const c=await client(); const originals=await Promise.all(['One','Two','Three'].map(t=>create(c,t)));
 const first=await c.call('/v1/board?limit=1');
 expect(first.timezone).toBe('Europe/London');expect(first.next_cursor).not.toBeNull();
 const changed=originals.find(t=>t.id!==first.tasks[0].id);
 await c.update(changed.id,{...mutation(1),changes:{title:'Changed',scheduled_date:'2020-02-29'}});
 await c.call('/v1/tasks/'+first.tasks[0].id,'DELETE',mutation(1));
 const extra=await create(c,'New');
 const read=[...first.tasks];let after=first.next_cursor;
 while(after){const p=await c.call(`/v1/board?limit=1&snapshot_cursor=${first.snapshot_cursor}&after=${after}`);read.push(...p.tasks);after=p.next_cursor;}
 expect(read.sort((a,b)=>a.id.localeCompare(b.id))).toEqual(originals.sort((a,b)=>a.id.localeCompare(b.id)));
 let cursor=first.snapshot_cursor;const events=[];
 for(;;){const p=await c.call(`/v1/changes?limit=1&after=${cursor}`);events.push(...p.events);expect(p.cursor).toBeGreaterThanOrEqual(cursor);cursor=p.cursor;if(p.next_cursor===null)break;}
 expect(events.map(e=>e.action)).toEqual(['update','delete','create']);
 expect(events.at(-1).after.id).toBe(extra.id);
 expect((await c.call('/v1/changes?after='+cursor)).events).toEqual([]);
 const exported=await c.call('/v1/export?limit=100');
 expect(exported.schema_version).toBe(1);expect(exported.tasks).toHaveLength(4);
 expect(exported.tasks.find(t=>t.id===first.tasks[0].id).deleted_at).not.toBeNull();
});
it('filters/pages reads, rejects malformed queries and never rolls dates forward',async()=>{
 const c=await client();const a=await create(c,'Invoice',{notes:'Keep COFFEE',scheduled_date:'2020-02-29'});
 await create(c,'Other');
 expect((await c.call('/v1/tasks?search=coffee&scheduled_date=2020-02-29&status=open')).tasks).toEqual([a]);
 expect((await c.call('/v1/tasks?undated=true')).tasks.map(t=>t.title)).toEqual(['Other']);
 const first=await c.call('/v1/tasks?limit=1');const next=await c.call('/v1/tasks?limit=1&after='+first.next_cursor);
 expect(next.tasks).toHaveLength(1);expect(next.tasks[0].id).not.toBe(first.tasks[0].id);expect(next.next_cursor).toBeNull();
 for(const q of ['limit=0','limit=101','limit=1&limit=2','undated=true&scheduled_date=2020-02-29','scheduled_date=2025-02-29','status=bad','secret=true']) await expect(c.call('/v1/tasks?'+q)).rejects.toMatchObject({status:400});
 await expect(c.call('/v1/board?after='+a.id)).rejects.toMatchObject({status:400});
 expect(await c.get(a.id)).toEqual(a);
});
it('reorders only within reviewed neighbors, preserves fields and appends when changing groups',async()=>{
 const c=await client();const a=await create(c,'A');const b=await create(c,'B');const d=await create(c,'D');
 const command={...mutation(1),after_id:a.id,before_id:b.id};
 const moved=(await c.call('/v1/tasks/'+d.id+'/reorder','POST',command)).task;
 expect((await c.list()).map(t=>t.id)).toEqual([a.id,d.id,b.id]);
 expect(moved).toMatchObject({revision:2,scheduled_date:null,title:'D'});
 expect((await c.call('/v1/tasks/'+d.id+'/reorder','POST',command)).task).toEqual(moved);
 await expect(c.call('/v1/tasks/'+b.id+'/reorder','POST',{...mutation(1),after_id:a.id,before_id:null})).rejects.toMatchObject({code:'ordering_conflict'});
 const dated=await create(c,'Dated',{scheduled_date:'2030-01-02'});
 await expect(c.call('/v1/tasks/'+b.id+'/reorder','POST',{...mutation(1),after_id:dated.id,before_id:null})).rejects.toMatchObject({code:'ordering_conflict'});
 const shifted=await c.update(d.id,{...mutation(2),changes:{scheduled_date:'2030-01-02'}});
 expect(shifted.position>dated.position).toBe(true);expect(shifted.scheduled_date).toBe('2030-01-02');
});
it('compacts old keys with history and revisions and rolls back all compaction changes on failure',async()=>{
 const c=await client();const a=await create(c,'A');const b=await create(c,'B');
 await env.DB.prepare('UPDATE tasks SET position=id WHERE owner_id=?').bind('github-12345').run();
 const old=await c.list();const newTask=await create(c,'New');
 expect((await c.list()).map(t=>t.id)).toEqual([...old.map(t=>t.id),newTask.id]);
 expect((await c.get(a.id)).revision).toBe(2);expect((await c.get(b.id)).revision).toBe(2);
 const history=await c.call('/v1/tasks/'+a.id+'/history');expect(history.events.at(-1).action).toBe('compact');
 expect(history.events.at(-1).after.position).toBe((await c.get(a.id)).position);
 await env.DB.prepare('UPDATE tasks SET position=id WHERE owner_id=?').bind('github-12345').run();
 const before=await c.list();const events=await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n');
 await env.DB.prepare("CREATE TRIGGER fail_receipt BEFORE UPDATE ON operation_receipts BEGIN SELECT RAISE(ABORT,'injected'); END").run();
 await expect(create(c,'Must fail')).rejects.toMatchObject({status:500});
 expect(await c.list()).toEqual(before);expect(await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n')).toBe(events);
});
it('rejects one competing insertion into the same reviewed gap without dropping either task',async()=>{
 const c=await client();const a=await create(c,'A');const b=await create(c,'B');const x=await create(c,'X');const y=await create(c,'Y');
 const before=await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n');
 const commands=[x,y].map(t=>({task:t,command:{...mutation(1),after_id:a.id,before_id:b.id}}));
 const results=await Promise.allSettled(commands.map(({task,command})=>c.call('/v1/tasks/'+task.id+'/reorder','POST',command)));
 expect(results.filter(r=>r.status==='fulfilled')).toHaveLength(1);
 expect(results.find(r=>r.status==='rejected').reason).toMatchObject({status:409,code:'ordering_conflict'});
 const tasks=await c.list();expect(tasks).toHaveLength(4);expect(new Set(tasks.map(t=>t.id)).size).toBe(4);
 const winner=results.findIndex(r=>r.status==='fulfilled');
 expect(tasks[1].id).toBe(commands[winner].task.id);
 expect((await c.call('/v1/tasks/'+commands[winner].task.id+'/reorder','POST',commands[winner].command)).task).toEqual(results[winner].value.task);
 expect(await env.DB.prepare('SELECT count(*) AS n FROM task_events').first('n')).toBe(before+1);
});
