package org.air2mi.app;
import android.content.*;
import android.database.*;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import java.io.*;
/** Only grants the system installer read access to the verified update APK. */
public final class UpdatePackageProvider extends ContentProvider {
 @Override public boolean onCreate(){return true;}
 private File file(Uri uri)throws FileNotFoundException{String component;
  if("/engine-update.apk".equals(uri.getPath()))component="engine";else if("/app-update.apk".equals(uri.getPath()))component="app";else throw new FileNotFoundException();
  if(uri.getQuery()!=null||uri.getFragment()!=null)throw new FileNotFoundException();return new File(getContext().getFilesDir(),"online-updates-"+component+"/"+component+"-update.apk");}
 @Override public ParcelFileDescriptor openFile(Uri uri,String mode)throws FileNotFoundException{if(!"r".equals(mode))throw new FileNotFoundException("read only");return ParcelFileDescriptor.open(file(uri),ParcelFileDescriptor.MODE_READ_ONLY);}
 @Override public String getType(Uri uri){return "application/vnd.android.package-archive";}
 @Override public Cursor query(Uri uri,String[] projection,String selection,String[] args,String sort){try{File f=file(uri);MatrixCursor c=new MatrixCursor(new String[]{OpenableColumns.DISPLAY_NAME,OpenableColumns.SIZE});c.addRow(new Object[]{"Air2Mi-update.apk",f.length()});return c;}catch(FileNotFoundException error){throw new IllegalArgumentException(error);}}
 @Override public Uri insert(Uri uri,ContentValues value){throw new UnsupportedOperationException();}
 @Override public int update(Uri uri,ContentValues value,String where,String[] args){throw new UnsupportedOperationException();}
 @Override public int delete(Uri uri,String where,String[] args){throw new UnsupportedOperationException();}
}
