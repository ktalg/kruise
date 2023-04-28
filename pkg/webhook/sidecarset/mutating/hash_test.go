package mutating

import (
	appsv1alpha1 "github.com/openkruise/kruise/apis/apps/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestSidecarSetHashWithoutImage(t *testing.T) {
	type args struct {
		sidecarSet *appsv1alpha1.SidecarSet
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "",
			args: args{
				sidecarSet: &appsv1alpha1.SidecarSet{
					TypeMeta:   metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{},
					Spec: appsv1alpha1.SidecarSetSpec{
						Containers: []appsv1alpha1.SidecarContainer{
							{
								Container: v1.Container{
									Name:  "sidecar1",
									Image: "nginx1.7.5",
									Command: []string{
										"sleep", "999d",
									},
									ImagePullPolicy: v1.PullAlways,
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "log-volume",
											MountPath: "/var/log",
										},
									},
									Resources:                v1.ResourceRequirements{},
									TerminationMessagePath:   "/dev/termination-log",
									TerminationMessagePolicy: v1.TerminationMessageReadFile,
								},
								PodInjectPolicy:   appsv1alpha1.BeforeAppContainerType,
								ShareVolumePolicy: appsv1alpha1.ShareVolumePolicy{Type: appsv1alpha1.ShareVolumePolicyDisabled},
								UpgradeStrategy:   appsv1alpha1.SidecarContainerUpgradeStrategy{UpgradeType: appsv1alpha1.SidecarContainerColdUpgrade},
							},
						},
					},
					Status: appsv1alpha1.SidecarSetStatus{},
				},
			},
			want:    "wcv47954b7xv5w4fbw682bbf78cw2449448vbf4fvvwv77947bc7fcv7fw89f98x",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//defaults.SetDefaultsSidecarSet(tt.args.sidecarSet)
			got, err := SidecarSetHashWithoutImage(tt.args.sidecarSet)
			if (err != nil) != tt.wantErr {
				t.Errorf("SidecarSetHashWithoutImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("SidecarSetHashWithoutImage() got = %v, want %v", got, tt.want)
			}
		})
	}
}
