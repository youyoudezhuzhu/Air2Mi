import{describe,it,expect}from'vitest';import{items,role,inputFromEngine,speakerFromEngine,inputPayload,speakerPayload}from'./engine';
describe('真实 API 边界',()=>{
 it('maps actual Registry names without manufacturing state',()=>{expect(inputFromEngine({input_id:'a',display_name:'客厅',enabled:true}).id).toBe('a');expect(speakerFromEngine({speaker_id:'b',display_name:'厨房',ip:'192.0.2.1'}).name).toBe('厨房');expect(items(undefined,'inputs')).toEqual([])});
 it('uses Engine source and references only',()=>{expect(role('b',{producer:'a',references:['a','b']})).toBe('备用音源');expect(role('a',{producer:'a',paused:true})).toBe('已暂停');expect(role('c',{producer:'a'})).toBe('无活动音频')});
 it('sends exact Engine model fields and never inserts volume or identity paths',()=>{expect(inputPayload({name:'A',enabled:true,speaker:'b'})).toEqual({display_name:'A',kind:'airplay',enabled:true,options:{speaker_id:'b'}});expect(inputPayload({name:'A',enabled:true,speaker:'b'},true)).not.toHaveProperty('input_id');expect(inputPayload({name:'A',enabled:true,speaker:'b'})).not.toHaveProperty('port');expect(speakerPayload({id:'b',name:'B',ip:'192.0.2.1',model:'',enabled:true},true)).not.toHaveProperty('volume')});
});

import{outputFor,outputLabel,type OutputState,type Snapshot}from'./engine';
it('shows output membership and playback from the API, independent of producer',()=>{const output={speaker_id:'b',selected:true,playback_state:'paused'} as OutputState;const s={state:{river:{producer:'a'},speakers:[output]}} as Snapshot;expect(outputLabel(outputFor(s,'b'))).toBe('已选中 · 已暂停');output.playback_state='playing';expect(outputLabel(outputFor(s,'b'))).toBe('正在播放');expect(outputLabel(outputFor(s,'unknown'))).toBe('状态暂不可用')});

import{associationOwner}from'./engine';it('reserves disabled receiver associations but keeps the current edit available',()=>{const inputs=[{id:'a',name:'A',enabled:false,options:{speaker_id:'s'}}];expect(associationOwner(inputs,'s')?.name).toBe('A');expect(associationOwner(inputs,'s','a')).toBeUndefined()});
it('reserves route-based associations as well as explicit options',()=>{expect(associationOwner([{id:'a',name:'A',enabled:true}],'s','b',[{route_id:'r',input_ids:['a'],speaker_ids:['s'],enabled:false}])?.id).toBe('a')});
