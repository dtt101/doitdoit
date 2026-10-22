import {env} from 'cloudflare:workers';
import {applyD1Migrations} from 'cloudflare:test';
import {it,expect} from 'vitest';
it('upgrades an existing database without losing acknowledgements, history, foreign keys or canonical task fields',async()=>{
 const db=env.UPGRADE_DB;await applyD1Migrations(db,env.TEST_MIGRATIONS.slice(0,3));
 await db.prepare("INSERT INTO owners VALUES('upgrade-owner','Europe/London')").run();
 const id=crypto.randomUUID(),operation=crypto.randomUUID(),now='2020-01-01T00:00:00Z';
 await db.prepare("INSERT INTO operation_receipts(owner_id,operation_id,request_hash,task_id,action,command_json,actor,accepted_at) VALUES(?,?,?,?,'create',?,?,?)")
  .bind('upgrade-owner',operation,'original-hash',id,JSON.stringify({title:'Retain',notes:'  notes\n',scheduled_date:'2020-02-29'}),'original-client',now).run();
 const before=await db.prepare('SELECT * FROM operation_receipts').first();const event=await db.prepare('SELECT * FROM task_events').first();const task=await db.prepare('SELECT * FROM tasks').first();
 await applyD1Migrations(db,env.TEST_MIGRATIONS.slice(3));
 expect(await db.prepare('SELECT * FROM operation_receipts').first()).toEqual(before);
 expect(await db.prepare('SELECT * FROM task_events').first()).toEqual(event);
 expect(await db.prepare('SELECT * FROM tasks').first()).toEqual(task);
 expect((await db.prepare('PRAGMA foreign_key_check').all()).results).toEqual([]);
 await db.prepare("INSERT INTO operation_receipts(owner_id,operation_id,request_hash,task_id,action,expected_revision,command_json,actor,accepted_at) VALUES(?,?,?,?,'delete',1,'{}',?,?)")
  .bind('upgrade-owner',crypto.randomUUID(),'delete-hash',id,'client',now).run();
 const deleted=await db.prepare('SELECT * FROM tasks').first();expect(deleted.revision).toBe(2);expect(deleted.deleted_at).toBe(now);expect(deleted.scheduled_date).toBe('2020-02-29');
 expect(await db.prepare('SELECT MAX(cursor) AS n FROM task_events').first('n')).toBeGreaterThan(event.cursor);
});
