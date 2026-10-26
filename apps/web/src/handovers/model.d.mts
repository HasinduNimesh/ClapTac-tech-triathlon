import type {TaskSnapshot} from '../tasks/loadTasks';
export type HandoverTask={id:string;priority:string;title:string;reason:string;reference:string;action:string;to:string;dueAt?:string};
export type Report={title:string;notes:string;asOf:string;tasks:HandoverTask[]};
export type Handover={id:string;role:string;scope:string;authorId:string;authorName:string;snapshot:Report;publishedAt:string;acknowledgedBy?:string;acknowledgedName?:string;acknowledgedAt?:string};
export function prepareHandover(data:TaskSnapshot,dispatcher:boolean,scope:string):Pick<Report,'asOf'|'tasks'>;
export function handoverCounts(tasks:HandoverTask[]):{total:number;critical:number;high:number;normal:number};
export function snapshotFresh(asOf:string,now:number):boolean;
