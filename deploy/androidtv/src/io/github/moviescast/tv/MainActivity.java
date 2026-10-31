package io.github.moviescast.tv;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.view.KeyEvent;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.window.OnBackInvokedDispatcher;

/**
 * Hosted app: the UI is the TV page of the movies-cast server (/tv/), the same
 * code the webOS app opens. The start page asks for the server address and
 * keeps it in localStorage; this shell turns load errors into the start page
 * and hands Back and the media keys to the page (window.moviesCastKey) with the
 * codes tv.js knows from the webOS remote; Android's media key codes differ.
 */
public class MainActivity extends Activity {
    static final String START = "file:///android_asset/start.html";
    static final String KODI = "org.xbmc.kodi";

    private WebView web;

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        // A film plays in a <video>; without this the TV's screen saver starts.
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        // A film sent from the phone starts without a key press on the TV.
        s.setMediaPlaybackRequiresUserGesture(false);
        web.setWebViewClient(new WebViewClient() {
            @Override
            public void onReceivedError(WebView view, WebResourceRequest req, WebResourceError err) {
                String url = req.getUrl().toString();
                if (!req.isForMainFrame() || url.startsWith("file:")) {
                    return;
                }
                view.loadUrl(START + "?fail=" + Uri.encode(String.valueOf(err.getDescription())));
            }
        });
        // Kodi plays the sound the WebView cannot (a tester's Shield, 2026-10-30:
        // AAC silent, no audio track choice); tv.js offers it when Kodi is installed.
        web.addJavascriptInterface(new Bridge(), "MoviesCastAndroid");
        setContentView(web);
        web.requestFocus();
        if (saved == null || web.restoreState(saved) == null) {
            web.loadUrl(START);
        }
        // From Android 13 on, Back goes to this callback, not to dispatchKeyEvent:
        // consuming the key there still closed the app (measured, Android 16).
        if (Build.VERSION.SDK_INT >= 33) {
            getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    OnBackInvokedDispatcher.PRIORITY_DEFAULT, this::pageBack);
        }
    }

    /** What tv.js may ask of Android: is Kodi there, start it, hand it a film. */
    class Bridge {
        @JavascriptInterface
        public boolean hasKodi() {
            return getPackageManager().getLaunchIntentForPackage(KODI) != null;
        }

        /** Starts Kodi, so its UPnP renderer answers the server's search. */
        @JavascriptInterface
        public boolean openKodi() {
            Intent i = getPackageManager().getLaunchIntentForPackage(KODI);
            if (i == null) {
                return false;
            }
            startActivity(i);
            return true;
        }

        /** Hands Kodi a stream URL, as other apps use Kodi as external player. */
        @JavascriptInterface
        public boolean playInKodi(String url, String mime, String title) {
            Intent i = new Intent(Intent.ACTION_VIEW);
            i.setDataAndType(Uri.parse(url), mime);
            i.setPackage(KODI);
            i.putExtra(Intent.EXTRA_TITLE, title);
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            try {
                startActivity(i);
                return true;
            } catch (ActivityNotFoundException e) {
                return false;
            }
        }
    }

    /** Back goes to the page first; the app closes when the page did not use it. */
    private void pageBack() {
        sendKey(461, true);
    }

    @Override
    @SuppressWarnings("deprecation")
    public void onBackPressed() {
        pageBack();
    }

    private void sendKey(int key, final boolean back) {
        // window.moviesCastKey answers true when the page used the key;
        // a page without it (start page, error page) answers false.
        web.evaluateJavascript(
            "(function(){try{return typeof moviesCastKey==='function'&&moviesCastKey(" + key + ")===true}catch(e){return false}})()",
            result -> {
                if (back && !"true".equals(result)) {
                    finish();
                }
            });
    }

    /** Media keys as the TV page knows them (tv.js KEY, the webOS remote's codes). */
    private static int pageKey(int keyCode) {
        switch (keyCode) {
            case KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE: return 10252;
            case KeyEvent.KEYCODE_MEDIA_PLAY: return 415;
            case KeyEvent.KEYCODE_MEDIA_PAUSE: return 19;
            case KeyEvent.KEYCODE_MEDIA_STOP: return 413;
            case KeyEvent.KEYCODE_MEDIA_FAST_FORWARD: return 417;
            case KeyEvent.KEYCODE_MEDIA_REWIND: return 412;
            default: return 0;
        }
    }

    @Override
    public boolean dispatchKeyEvent(KeyEvent ev) {
        int key = pageKey(ev.getKeyCode());
        if (key == 0) {
            return super.dispatchKeyEvent(ev);
        }
        if (ev.getAction() == KeyEvent.ACTION_DOWN && ev.getRepeatCount() == 0) {
            sendKey(key, false);
        }
        return true;
    }

    @Override
    protected void onPause() {
        super.onPause();
        web.onPause();
    }

    @Override
    protected void onResume() {
        super.onResume();
        web.onResume();
    }

    @Override
    protected void onSaveInstanceState(Bundle out) {
        super.onSaveInstanceState(out);
        web.saveState(out);
    }

    @Override
    protected void onDestroy() {
        web.destroy();
        super.onDestroy();
    }
}
