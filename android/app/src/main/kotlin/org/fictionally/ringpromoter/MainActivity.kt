package org.fictionally.ringpromoter

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.Surface
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import org.fictionally.ringpromoter.ui.RingPromoterApp
import org.fictionally.ringpromoter.ui.theme.RingPromoterTheme

class MainActivity : ComponentActivity() {

    private val container by lazy { AppContainer(applicationContext) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
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
}
