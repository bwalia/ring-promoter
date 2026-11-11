# kotlinx.serialization keeps its generated serializers via @Serializable, but
# R8 needs the companions and the serializer fields left alone.
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**
-keepclassmembers class org.fictionally.ringpromoter.** {
    *** Companion;
}
-keepclasseswithmembers class org.fictionally.ringpromoter.** {
    kotlinx.serialization.KSerializer serializer(...);
}
