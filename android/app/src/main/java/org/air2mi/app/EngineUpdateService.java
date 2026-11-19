package org.air2mi.app;
import android.app.*;
import android.content.*;
import android.content.pm.ServiceInfo;
import java.util.concurrent.*;
/** Foreground APK download; independent from audio playback lifetime. */
public final class EngineUpdateService extends Service {
 private final ExecutorService worker=Executors.newSingleThreadExecutor();
 @Override public void onCreate(){super.onCreate();getSystemService(NotificationManager.class).createNotificationChannel(new NotificationChannel("air2mi-update","Air2Mi 更新",NotificationManager.IMPORTANCE_LOW));
  PendingIntent open=PendingIntent.getActivity(this,0,new Intent(this,MainActivity.class),PendingIntent.FLAG_IMMUTABLE|PendingIntent.FLAG_UPDATE_CURRENT);
  startForeground(42,new Notification.Builder(this,"air2mi-update").setSmallIcon(android.R.drawable.stat_sys_download).setContentTitle("Air2Mi 更新").setContentText("正在下载并校验更新，完成后请打开应用确认系统安装").setContentIntent(open).setOngoing(true).build(),ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC);
 }
 @Override public int onStartCommand(Intent intent,int flags,int id){String component=intent==null?"engine":intent.getStringExtra("component");worker.execute(()->{try{EngineRuntime.get(this).onlineUpdates(component).download();}catch(Exception error){android.util.Log.w("Air2Mi","更新下载失败");}finally{stopForeground(STOP_FOREGROUND_REMOVE);stopSelfResult(id);}});return START_NOT_STICKY;}
 @Override public android.os.IBinder onBind(Intent intent){return null;}
 @Override public void onDestroy(){worker.shutdown();super.onDestroy();}
}
