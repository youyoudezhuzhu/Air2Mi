import{ref}from'vue';
export const confirmation=ref<{title:string;message:string;confirmLabel:string;cancelLabel:string;resolve:(v:boolean)=>void}>();
export function confirmAction(title:string,message:string,confirmLabel='继续',cancelLabel='取消'){if(confirmation.value)confirmation.value.resolve(false);return new Promise<boolean>(resolve=>confirmation.value={title,message,confirmLabel,cancelLabel,resolve})}
export function answerConfirmation(answer:boolean){const value=confirmation.value;confirmation.value=undefined;value?.resolve(answer)}
