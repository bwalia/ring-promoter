package org.fictionally.ringpromoter

import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.fictionally.ringpromoter.ui.RingPromoterApp
import org.fictionally.ringpromoter.ui.theme.RingPromoterTheme

/**
 * A [FragmentActivity] rather than a plain `ComponentActivity`: that is what
 * `BiometricPrompt` requires to host its own sheet.
 */
class MainActivity : FragmentActivity() {

    private val container by lazy { AppContainer(applicationContext) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        container.open(intent?.dataString)
        setContent {
            val theme by container.settings.theme.collectAsStateWithLifecycle()
            CompositionLocalProvider(LocalContainer provides container) {
                RingPromoterTheme(choice = theme) {
                    Surface(modifier = Modifier.fillMaxSize()) {
                        RingPromoterApp()
                    }
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        container.open(intent.dataString)
    }

    /**
     * Lock on the way out, so the app is already covered when it comes back
     * rather than showing a frame of the previous screen first.
     */
    override fun onStop() {
        super.onStop()
        if (container.settings.lockOnOpen.value) container.biometrics.lock()
    }
}
